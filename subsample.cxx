#include <algorithm>
#include <cstdint>
#include <cstdlib>
#include <filesystem>
#include <iostream>
#include <memory>
#include <numeric>
#include <random>
#include <stdexcept>
#include <string>
#include <unordered_set>
#include <utility>
#include <vector>

#include "TBranch.h"
#include "TDirectory.h"
#include "TEntryList.h"
#include "TFile.h"
#include "TFileMerger.h"
#include "TKey.h"
#include "TLeaf.h"
#include "TObject.h"
#include "TROOT.h"
#include "TTree.h"

namespace fs = std::filesystem;

[[noreturn]] static void fail(const std::string& msg) { throw std::runtime_error(msg); }

struct TreeInfo {
  std::string path;
  int entries = 0;
};

struct EventKey {
  int pathIdx = 0;
  std::uint64_t collisionBits = 0;

  bool operator==(const EventKey& other) const {
    return pathIdx == other.pathIdx && collisionBits == other.collisionBits;
  }
};

struct EventKeyHash {
  std::size_t operator()(const EventKey& k) const noexcept {
    std::size_t h1 = std::hash<int>{}(k.pathIdx);
    std::size_t h2 = std::hash<std::uint64_t>{}(k.collisionBits);
    return h1 ^ (h2 + 0x9e3779b97f4a7c15ULL + (h1 << 6U) + (h1 >> 2U));
  }
};

using EventKeySet = std::unordered_set<EventKey, EventKeyHash>;

enum class CollisionLeafKind { kSigned, kUnsigned };

struct CollisionLeafReader {
  TLeaf* leaf = nullptr;
  CollisionLeafKind kind = CollisionLeafKind::kSigned;
};

struct Config {
  std::string outputFile;
  std::vector<std::string> inputFiles;
  std::string observationsTree = "O2pidtracksmc";
  std::string collisionIdBranch = "fIndexCollisions";
  int seed = 12345;

  int byTracks = -1;             // Row-level uniform cap over all files.
  int byEvents = -1;             // Distinct event groups cap, row filtering.
  int byDataframesUpToEvents = -1;  // Distinct event groups cap, whole-DF selection.
};

static void printUsage() {
  std::cerr << "Usage:\n"
            << "  subsample [options] output.root input1.root [input2.root ...]\n\n"
            << "Sampling options (pick at most one):\n"
            << "  --by-tracks N                  Uniform row-level subsample by observation count\n"
            << "  --by-events N                  Subsample by distinct event IDs, keep matching rows\n"
            << "  --by-dataframes-up-to-events N Subsample by whole DataFrames using event-count budget\n\n"
            << "Shared options:\n"
            << "  --tree-name NAME          Observations tree name (default: O2pidtracksmc)\n"
            << "  --event-id-branch NAME    Event ID branch (default: fIndexCollisions)\n"
            << "  --seed N                  RNG seed (default: 12345)\n";
}

static std::unique_ptr<TFile> openRootRead(const std::string& path) {
  TFile* raw = TFile::Open(path.c_str(), "READ");
  if (!raw || raw->IsZombie()) fail("Failed to open input file: " + path);
  return std::unique_ptr<TFile>(raw);
}

static std::unique_ptr<TFile> openRootRecreate(const std::string& path) {
  TFile* raw = TFile::Open(path.c_str(), "RECREATE");
  if (!raw || raw->IsZombie()) fail("Failed to create output file: " + path);
  return std::unique_ptr<TFile>(raw);
}

static std::vector<std::string> splitPath(const std::string& p) {
  std::vector<std::string> parts;
  std::string cur;
  for (char c : p) {
    if (c == '/') {
      if (!cur.empty()) parts.push_back(cur);
      cur.clear();
    } else {
      cur.push_back(c);
    }
  }
  if (!cur.empty()) parts.push_back(cur);
  return parts;
}

static TDirectory* mkdirP(TDirectory* parent, const std::vector<std::string>& parts) {
  TDirectory* d = parent;
  for (const auto& part : parts) {
    TObject* existing = d ? d->Get(part.c_str()) : nullptr;
    if (existing && existing->InheritsFrom("TDirectory")) {
      d = dynamic_cast<TDirectory*>(existing);
      continue;
    }
    d = d->mkdir(part.c_str());
  }
  return d;
}

static void writeTreeByPath(TDirectory* outFile, TTree* tree, const std::string& fullPath) {
  auto parts = splitPath(fullPath);
  if (parts.empty()) fail("Invalid tree path: " + fullPath);
  const std::string localName = parts.back();
  parts.pop_back();
  TDirectory* targetDir = parts.empty() ? outFile : mkdirP(outFile, parts);
  targetDir->cd();
  tree->SetName(localName.c_str());
  tree->Write(localName.c_str());
}

static void copyParentFilesObject(TFile& src, TFile& dst) {
  TObject* parentFiles = src.Get("parentFiles");
  if (!parentFiles) return;
  dst.cd();
  if (TObject* clone = parentFiles->Clone()) clone->Write("parentFiles");
}

static void collectTreePathsRecursive(TObject* obj,
                                      const std::string& prefix,
                                      const std::string& localTreeName,
                                      std::vector<TreeInfo>& out) {
  if (!obj) return;
  if (obj->InheritsFrom("TDirectory")) {
    auto* dir = dynamic_cast<TDirectory*>(obj);
    if (!dir) return;
    TIter it(dir->GetListOfKeys());
    while (auto* key = dynamic_cast<TKey*>(it())) {
      const std::string name = key->GetName();
      std::unique_ptr<TObject> sub(key->ReadObj());
      const std::string next = prefix.empty() ? name : (prefix + "/" + name);
      collectTreePathsRecursive(sub.get(), next, localTreeName, out);
    }
    return;
  }
  if (obj->InheritsFrom("TTree")) {
    auto* t = dynamic_cast<TTree*>(obj);
    if (t && std::string(t->GetName()) == localTreeName) {
      out.push_back(TreeInfo{prefix, static_cast<int>(t->GetEntries())});
    }
  }
}

static std::vector<TreeInfo> collectTreeInfos(TFile& f, const std::string& localTreeName) {
  std::vector<TreeInfo> out;
  collectTreePathsRecursive(&f, "", localTreeName, out);
  return out;
}

static std::vector<int> computeUniformCounts(const std::vector<int>& entriesPerFile, int targetTotal) {
  const int totalAvailable = std::accumulate(entriesPerFile.begin(), entriesPerFile.end(), 0);
  targetTotal = std::max(0, std::min(targetTotal, totalAvailable));

  std::vector<int> counts(entriesPerFile.size(), 0);
  int budget = targetTotal;
  std::vector<int> remaining(entriesPerFile.size());
  std::iota(remaining.begin(), remaining.end(), 0);

  while (!remaining.empty() && budget > 0) {
    const int per = budget / static_cast<int>(remaining.size());
    const int rem = budget % static_cast<int>(remaining.size());
    std::vector<int> newRemaining;
    newRemaining.reserve(remaining.size());

    for (std::size_t pos = 0; pos < remaining.size(); ++pos) {
      const int i = remaining[pos];
      const int desired = per + (static_cast<int>(pos) < rem ? 1 : 0);
      if (desired >= entriesPerFile[i]) {
        counts[i] = entriesPerFile[i];
        budget -= entriesPerFile[i];
      } else {
        counts[i] += desired;
        budget -= desired;
        newRemaining.push_back(i);
      }
    }
    remaining.swap(newRemaining);
  }

  return counts;
}

static std::vector<int> allocateProportionalCounts(int targetTotal, const std::vector<int>& caps) {
  if (targetTotal <= 0) return std::vector<int>(caps.size(), 0);
  const int totalCap = std::accumulate(caps.begin(), caps.end(), 0);
  if (totalCap <= 0) return std::vector<int>(caps.size(), 0);
  if (targetTotal >= totalCap) return caps;

  std::vector<double> desired(caps.size(), 0.0);
  std::vector<int> base(caps.size(), 0);
  for (std::size_t i = 0; i < caps.size(); ++i) {
    desired[i] = static_cast<double>(targetTotal) * static_cast<double>(caps[i]) / static_cast<double>(totalCap);
    base[i] = std::min(caps[i], static_cast<int>(desired[i]));
  }
  int remainder = targetTotal - std::accumulate(base.begin(), base.end(), 0);
  if (remainder <= 0) return base;

  std::vector<int> order(caps.size());
  std::iota(order.begin(), order.end(), 0);
  std::sort(order.begin(), order.end(), [&](int a, int b) {
    const double fa = desired[a] - static_cast<double>(base[a]);
    const double fb = desired[b] - static_cast<double>(base[b]);
    if (fa != fb) return fa > fb;
    return a < b;
  });

  std::vector<int> out = base;
  for (int i : order) {
    if (remainder <= 0) break;
    if (out[i] < caps[i]) {
      out[i] += 1;
      --remainder;
    }
  }
  return out;
}

static std::vector<int> sampleIndicesWithoutReplacement(std::mt19937_64& rng, int n, int k) {
  if (k <= 0) return {};
  if (k >= n) {
    std::vector<int> all(n);
    std::iota(all.begin(), all.end(), 0);
    return all;
  }
  std::unordered_set<int> picks;
  picks.reserve(static_cast<std::size_t>(k) * 2U);
  std::uniform_int_distribution<int> dist(0, n - 1);
  while (static_cast<int>(picks.size()) < k) picks.insert(dist(rng));
  std::vector<int> out(picks.begin(), picks.end());
  std::sort(out.begin(), out.end());
  return out;
}

static CollisionLeafReader bindCollisionLeaf(TTree& t, const std::string& branchName) {
  TBranch* br = t.GetBranch(branchName.c_str());
  if (!br) fail("Branch '" + branchName + "' not found in tree '" + std::string(t.GetName()) + "'");
  TLeaf* leaf = br->GetLeaf(branchName.c_str());
  if (!leaf) fail("Could not get leaf for branch '" + branchName + "' in tree '" + std::string(t.GetName()) + "'");
  const std::string tn = leaf->GetTypeName();

  CollisionLeafReader r{};
  r.leaf = leaf;
  t.SetBranchStatus("*", 0);
  t.SetBranchStatus(branchName.c_str(), 1);

  if (tn == "Int_t" || tn == "int" || tn == "int32_t" || tn == "Long64_t" || tn == "Long_t" || tn == "long long" ||
      tn == "int64_t") {
    r.kind = CollisionLeafKind::kSigned;
    return r;
  }
  if (tn == "UInt_t" || tn == "unsigned int" || tn == "uint32_t" || tn == "ULong64_t" || tn == "ULong_t" ||
      tn == "unsigned long long" || tn == "uint64_t") {
    r.kind = CollisionLeafKind::kUnsigned;
    return r;
  }
  fail("Branch '" + branchName + "' has scalar type '" + tn + "', not supported (need integer)");
}

static std::uint64_t collisionBitsFromReader(const CollisionLeafReader& r) {
  if (!r.leaf) fail("Internal error: collision leaf reader not initialized");
  if (r.kind == CollisionLeafKind::kSigned) {
    const auto v = static_cast<Long64_t>(r.leaf->GetValueLong64(0));
    return static_cast<std::uint64_t>(v);
  }
  return static_cast<std::uint64_t>(static_cast<ULong64_t>(r.leaf->GetValueLong64(0)));
}

static void mergeRootFiles(const std::string& outputFile,
                           const std::vector<std::string>& inputFiles,
                           const std::string& warningPrefix = "file") {
  TFileMerger merger;
  merger.SetFastMethod(true);
  merger.OutputFile(outputFile.c_str());
  for (const auto& f : inputFiles) {
    if (!merger.AddFile(f.c_str())) std::cout << "Warning: could not add " << warningPrefix << " " << f << "\n";
  }
  if (!merger.Merge()) fail("Merging failed.");
}

static std::string subsetPath(const fs::path& tmpDir, int index) {
  return (tmpDir / ("subset_" + std::to_string(index) + ".root")).string();
}

// Mode 1: row-uniform observation cap.
static void runObservationMode(const Config& cfg) {
  std::vector<int> entriesPerFile;
  entriesPerFile.reserve(cfg.inputFiles.size());
  for (const auto& filePath : cfg.inputFiles) {
    auto f = openRootRead(filePath);
    const auto infos = collectTreeInfos(*f, cfg.observationsTree);
    const int total = std::accumulate(infos.begin(), infos.end(), 0, [](int acc, const TreeInfo& ti) { return acc + ti.entries; });
    if (total <= 0) fail("Input file '" + filePath + "' has no observations tree '" + cfg.observationsTree + "'");
    entriesPerFile.push_back(total);
  }

  const int totalAvailable = std::accumulate(entriesPerFile.begin(), entriesPerFile.end(), 0);
  const auto fileBudgets = computeUniformCounts(entriesPerFile, cfg.byTracks);
  const int selectedTotal = std::accumulate(fileBudgets.begin(), fileBudgets.end(), 0);
  std::cout << "Observation mode: target=" << cfg.byTracks << ", available=" << totalAvailable
            << ", selected=" << selectedTotal << "\n";

  if (selectedTotal == totalAvailable) {
    mergeRootFiles(cfg.outputFile, cfg.inputFiles);
    return;
  }

  fs::path tmpDir = fs::temp_directory_path() / fs::path("obs_subsets_cpp_" + std::to_string(std::rand()));
  fs::create_directories(tmpDir);
  std::vector<std::string> subsetFiles;

  try {
    for (std::size_t i = 0; i < cfg.inputFiles.size(); ++i) {
      const int budget = fileBudgets[i];
      if (budget <= 0) continue;

      const std::string inPath = cfg.inputFiles[i];
      const std::string outPath = subsetPath(tmpDir, static_cast<int>(i));
      auto fin = openRootRead(inPath);
      auto fout = openRootRecreate(outPath);
      const auto infos = collectTreeInfos(*fin, cfg.observationsTree);
      if (infos.empty()) fail("No tree '" + cfg.observationsTree + "' in " + inPath);
      const std::vector<int> caps = [&]() {
        std::vector<int> out;
        out.reserve(infos.size());
        for (const auto& ti : infos) out.push_back(ti.entries);
        return out;
      }();
      const auto treeBudgets = allocateProportionalCounts(budget, caps);

      int written = 0;
      for (std::size_t tIdx = 0; tIdx < infos.size(); ++tIdx) {
        const auto& ti = infos[tIdx];
        const int nSel = treeBudgets[tIdx];
        if (nSel <= 0) continue;
        TTree* t = dynamic_cast<TTree*>(fin->Get(ti.path.c_str()));
        if (!t) fail("Missing tree at '" + ti.path + "' in '" + inPath + "'");

        std::mt19937_64 rng(static_cast<std::uint64_t>(cfg.seed) + static_cast<std::uint64_t>(i) * 10007ULL +
                            static_cast<std::uint64_t>(tIdx) * 1000003ULL);
        const auto picks = sampleIndicesWithoutReplacement(rng, ti.entries, nSel);
        auto* entryList = new TEntryList(("elist_obs_" + std::to_string(i) + "_" + std::to_string(tIdx)).c_str(), "");
        for (int idx : picks) entryList->Enter(idx);
        t->SetEntryList(entryList);
        TTree* tSub = t->CopyTree("");
        if (!tSub) fail("CopyTree failed for '" + ti.path + "'");
        written += static_cast<int>(tSub->GetEntries());
        writeTreeByPath(fout.get(), tSub, ti.path);
        t->SetEntryList(nullptr);
        tSub->SetDirectory(nullptr);
        delete tSub;
        delete entryList;
      }
      if (written != budget) fail("Subsampling mismatch in '" + inPath + "'");
      subsetFiles.push_back(outPath);
    }
    if (subsetFiles.empty()) fail("No subset files were created.");
    mergeRootFiles(cfg.outputFile, subsetFiles, "subset");
  } catch (...) {
    for (const auto& p : subsetFiles) {
      std::error_code ec;
      fs::remove(p, ec);
    }
    std::error_code ec;
    fs::remove_all(tmpDir, ec);
    throw;
  }

  for (const auto& p : subsetFiles) {
    std::error_code ec;
    fs::remove(p, ec);
  }
  std::error_code ec;
  fs::remove_all(tmpDir, ec);
}

struct EventScanResult {
  std::vector<std::string> internalPaths;
  std::vector<int> internalEntries;
  EventKeySet keys;
};

static EventScanResult scanFileEvents(const std::string& inputFile,
                                      const std::string& treeName,
                                      const std::string& eventIdBranch) {
  auto fin = openRootRead(inputFile);
  const auto infos = collectTreeInfos(*fin, treeName);
  if (infos.empty()) fail("No tree '" + treeName + "' in file " + inputFile);

  EventScanResult out;
  out.internalPaths.reserve(infos.size());
  out.internalEntries.reserve(infos.size());

  for (const auto& ti : infos) {
    out.internalPaths.push_back(ti.path);
    out.internalEntries.push_back(ti.entries);
  }

  for (std::size_t pathIdx = 0; pathIdx < infos.size(); ++pathIdx) {
    const auto& ti = infos[pathIdx];
    TTree* t = dynamic_cast<TTree*>(fin->Get(ti.path.c_str()));
    if (!t) fail("Missing tree '" + ti.path + "' in " + inputFile);
    auto reader = bindCollisionLeaf(*t, eventIdBranch);
    for (int row = 0; row < ti.entries; ++row) {
      t->GetEntry(row);
      out.keys.insert(EventKey{static_cast<int>(pathIdx), collisionBitsFromReader(reader)});
    }
    t->ResetBranchAddresses();
  }
  return out;
}

// Mode 2: event-group row filtering (old per_event behavior).
static void runEventGroupMode(const Config& cfg) {
  std::vector<int> eventsPerFile;
  eventsPerFile.reserve(cfg.inputFiles.size());
  for (const auto& filePath : cfg.inputFiles) {
    auto scan = scanFileEvents(filePath, cfg.observationsTree, cfg.collisionIdBranch);
    const int nEvents = static_cast<int>(scan.keys.size());
    if (nEvents <= 0) fail("No events found in '" + filePath + "'");
    eventsPerFile.push_back(nEvents);
  }

  const auto fileBudgets = computeUniformCounts(eventsPerFile, cfg.byEvents);
  const int totalAvailable = std::accumulate(eventsPerFile.begin(), eventsPerFile.end(), 0);
  const int selectedTotal = std::accumulate(fileBudgets.begin(), fileBudgets.end(), 0);
  std::cout << "Event-group mode: target=" << cfg.byEvents << ", available=" << totalAvailable
            << ", selected=" << selectedTotal << "\n";

  if (selectedTotal == totalAvailable) {
    mergeRootFiles(cfg.outputFile, cfg.inputFiles);
    return;
  }

  fs::path tmpDir = fs::temp_directory_path() / fs::path("event_subsets_cpp_" + std::to_string(std::rand()));
  fs::create_directories(tmpDir);
  std::vector<std::string> subsetFiles;

  try {
    for (std::size_t i = 0; i < cfg.inputFiles.size(); ++i) {
      const int budget = fileBudgets[i];
      if (budget <= 0) continue;

      const std::string inPath = cfg.inputFiles[i];
      auto scan = scanFileEvents(inPath, cfg.observationsTree, cfg.collisionIdBranch);
      std::vector<EventKey> allKeys(scan.keys.begin(), scan.keys.end());
      std::sort(allKeys.begin(), allKeys.end(), [](const EventKey& a, const EventKey& b) {
        if (a.pathIdx != b.pathIdx) return a.pathIdx < b.pathIdx;
        return a.collisionBits < b.collisionBits;
      });

      std::mt19937_64 rng(static_cast<std::uint64_t>(cfg.seed) + static_cast<std::uint64_t>(i) * 10007ULL);
      const auto pickedIdx = sampleIndicesWithoutReplacement(rng, static_cast<int>(allKeys.size()), budget);
      EventKeySet selectedKeys;
      selectedKeys.reserve(static_cast<std::size_t>(budget) * 2U);
      for (int idx : pickedIdx) selectedKeys.insert(allKeys[static_cast<std::size_t>(idx)]);

      const std::string outPath = subsetPath(tmpDir, static_cast<int>(i));
      auto fin = openRootRead(inPath);
      auto fout = openRootRecreate(outPath);
      copyParentFilesObject(*fin, *fout);

      for (std::size_t tIdx = 0; tIdx < scan.internalPaths.size(); ++tIdx) {
        const std::string& treePath = scan.internalPaths[tIdx];
        TTree* t = dynamic_cast<TTree*>(fin->Get(treePath.c_str()));
        if (!t) fail("Could not retrieve tree at path '" + treePath + "'");
        auto reader = bindCollisionLeaf(*t, cfg.collisionIdBranch);
        auto* entryList = new TEntryList(("elist_evt_" + std::to_string(i) + "_" + std::to_string(tIdx)).c_str(), "");

        for (int row = 0; row < scan.internalEntries[tIdx]; ++row) {
          t->GetEntry(row);
          if (selectedKeys.find(EventKey{static_cast<int>(tIdx), collisionBitsFromReader(reader)}) != selectedKeys.end()) {
            entryList->Enter(static_cast<Long64_t>(row));
          }
        }

        t->SetBranchStatus("*", 1);
        t->SetEntryList(entryList);
        TTree* tSub = t->CopyTree("");
        if (!tSub) fail("CopyTree failed for '" + treePath + "'");
        writeTreeByPath(fout.get(), tSub, treePath);
        t->SetEntryList(nullptr);
        t->ResetBranchAddresses();
        tSub->SetDirectory(nullptr);
        delete tSub;
        delete entryList;
      }
      subsetFiles.push_back(outPath);
    }

    if (subsetFiles.empty()) fail("No subset files were created.");
    mergeRootFiles(cfg.outputFile, subsetFiles, "subset");
  } catch (...) {
    for (const auto& p : subsetFiles) {
      std::error_code ec;
      fs::remove(p, ec);
    }
    std::error_code ec;
    fs::remove_all(tmpDir, ec);
    throw;
  }

  for (const auto& p : subsetFiles) {
    std::error_code ec;
    fs::remove(p, ec);
  }
  std::error_code ec;
  fs::remove_all(tmpDir, ec);
}

struct TreeMeta {
  std::string path;
  int entries = 0;
  int distinctEvents = 0;
};

struct FileMeta {
  std::string filePath;
  std::vector<TreeMeta> trees;
  int totalEvents = 0;
};

static int distinctEventsInTree(TTree& t, const std::string& eventIdBranch) {
  auto reader = bindCollisionLeaf(t, eventIdBranch);
  std::unordered_set<std::uint64_t> seen;
  seen.reserve(static_cast<std::size_t>(std::max<Long64_t>(1024, t.GetEntries() / 4)));
  const Long64_t n = t.GetEntries();
  for (Long64_t i = 0; i < n; ++i) {
    t.GetEntry(i);
    seen.insert(collisionBitsFromReader(reader));
  }
  t.SetBranchStatus("*", 1);
  return static_cast<int>(seen.size());
}

static std::vector<FileMeta> scanDataframes(const Config& cfg) {
  std::vector<FileMeta> out;
  out.reserve(cfg.inputFiles.size());
  for (const auto& filePath : cfg.inputFiles) {
    auto f = openRootRead(filePath);
    const auto infos = collectTreeInfos(*f, cfg.observationsTree);
    if (infos.empty()) fail("No tree '" + cfg.observationsTree + "' in file " + filePath);
    FileMeta fm;
    fm.filePath = filePath;
    for (const auto& ti : infos) {
      TTree* t = dynamic_cast<TTree*>(f->Get(ti.path.c_str()));
      if (!t) fail("Failed reading tree '" + ti.path + "' from " + filePath);
      TreeMeta tm;
      tm.path = ti.path;
      tm.entries = ti.entries;
      tm.distinctEvents = distinctEventsInTree(*t, cfg.collisionIdBranch);
      if (tm.distinctEvents > 0) {
        fm.totalEvents += tm.distinctEvents;
        fm.trees.push_back(std::move(tm));
      }
    }
    if (fm.totalEvents <= 0) fail("No distinct events in file " + filePath);
    out.push_back(std::move(fm));
  }
  return out;
}

// Mode 3: whole-dataframe selection by event budget (old per_dataframe behavior).
static void runDataframeMode(const Config& cfg) {
  const auto scanned = scanDataframes(cfg);
  std::vector<int> caps;
  caps.reserve(scanned.size());
  for (const auto& fm : scanned) caps.push_back(fm.totalEvents);
  const auto fileBudgets = allocateProportionalCounts(cfg.byDataframesUpToEvents, caps);

  const int totalAvailable = std::accumulate(caps.begin(), caps.end(), 0);
  const int selectedTotal = std::accumulate(fileBudgets.begin(), fileBudgets.end(), 0);
  std::cout << "Dataframe mode: target=" << cfg.byDataframesUpToEvents << ", allocated=" << selectedTotal
            << ", available=" << totalAvailable << "\n";

  fs::path tmpDir = fs::temp_directory_path() / fs::path("df_subsets_cpp_" + std::to_string(std::rand()));
  fs::create_directories(tmpDir);
  std::vector<std::string> subsetFiles;

  try {
    for (std::size_t i = 0; i < scanned.size(); ++i) {
      if (fileBudgets[i] <= 0) continue;
      auto trees = scanned[i].trees;
      std::mt19937_64 rng(static_cast<std::uint64_t>(cfg.seed) + static_cast<std::uint64_t>(i) * 10007ULL);
      std::shuffle(trees.begin(), trees.end(), rng);

      std::vector<TreeMeta> selectedTrees;
      int sumEvents = 0;
      for (const auto& tm : trees) {
        if (sumEvents >= fileBudgets[i]) break;
        selectedTrees.push_back(tm);
        sumEvents += tm.distinctEvents;
      }
      if (selectedTrees.empty()) continue;

      std::cout << "[" << i << "] " << scanned[i].filePath << ": target_events=" << fileBudgets[i]
                << ", selected_df=" << selectedTrees.size() << ", selected_events=" << sumEvents << "\n";

      const std::string outPath = subsetPath(tmpDir, static_cast<int>(i));
      auto fin = openRootRead(scanned[i].filePath);
      auto fout = openRootRecreate(outPath);
      copyParentFilesObject(*fin, *fout);
      for (const auto& tm : selectedTrees) {
        TTree* t = dynamic_cast<TTree*>(fin->Get(tm.path.c_str()));
        if (!t) fail("Missing tree '" + tm.path + "'");
        std::unique_ptr<TTree> copy(t->CloneTree(-1, "fast"));
        if (!copy) fail("CloneTree failed for '" + tm.path + "'");
        writeTreeByPath(fout.get(), copy.get(), tm.path);
      }
      subsetFiles.push_back(outPath);
    }

    if (subsetFiles.empty()) fail("No subset files created.");
    mergeRootFiles(cfg.outputFile, subsetFiles, "subset");
  } catch (...) {
    for (const auto& p : subsetFiles) {
      std::error_code ec;
      fs::remove(p, ec);
    }
    std::error_code ec;
    fs::remove_all(tmpDir, ec);
    throw;
  }

  for (const auto& p : subsetFiles) {
    std::error_code ec;
    fs::remove(p, ec);
  }
  std::error_code ec;
  fs::remove_all(tmpDir, ec);
}

static Config parseArgs(int argc, char** argv) {
  Config cfg;
  for (int i = 1; i < argc; ++i) {
    std::string a = argv[i];
    if (a == "--by-tracks") {
      if (++i >= argc) fail("Missing value for --by-tracks");
      cfg.byTracks = std::stoi(argv[i]);
      continue;
    }
    if (a == "--by-events") {
      if (++i >= argc) fail("Missing value for --by-events");
      cfg.byEvents = std::stoi(argv[i]);
      continue;
    }
    if (a == "--by-dataframes-up-to-events") {
      if (++i >= argc) fail("Missing value for --by-dataframes-up-to-events");
      cfg.byDataframesUpToEvents = std::stoi(argv[i]);
      continue;
    }
    if (a == "--tree-name") {
      if (++i >= argc) fail("Missing value for --tree-name");
      cfg.observationsTree = argv[i];
      continue;
    }
    if (a == "--event-id-branch") {
      if (++i >= argc) fail("Missing value for --event-id-branch");
      cfg.collisionIdBranch = argv[i];
      continue;
    }
    if (a == "--seed") {
      if (++i >= argc) fail("Missing value for --seed");
      cfg.seed = std::stoi(argv[i]);
      continue;
    }
    if (!a.empty() && a[0] == '-') fail("Unknown option: " + a);
    if (cfg.outputFile.empty()) cfg.outputFile = a;
    else cfg.inputFiles.push_back(a);
  }

  if (cfg.outputFile.empty() || cfg.inputFiles.empty()) {
    printUsage();
    fail("Need output file and at least one input file.");
  }

  const int selectedModes = static_cast<int>(cfg.byTracks >= 0) + static_cast<int>(cfg.byEvents >= 0) +
                            static_cast<int>(cfg.byDataframesUpToEvents >= 0);
  if (selectedModes > 1) {
    fail("Sampling flags are mutually exclusive.");
  }
  return cfg;
}

int main(int argc, char** argv) {
  try {
    gROOT->SetBatch(true);
    const Config cfg = parseArgs(argc, argv);

    if (cfg.byDataframesUpToEvents >= 0) {
      runDataframeMode(cfg);
    } else if (cfg.byEvents >= 0) {
      runEventGroupMode(cfg);
    } else if (cfg.byTracks >= 0) {
      runObservationMode(cfg);
    } else {
      for (const auto& f : cfg.inputFiles) std::cout << "Adding: " << f << "\n";
      mergeRootFiles(cfg.outputFile, cfg.inputFiles);
      std::cout << "Merged " << cfg.inputFiles.size() << " files into " << cfg.outputFile << "\n";
    }
    return 0;
  } catch (const std::exception& ex) {
    std::cerr << ex.what() << "\n";
    return 1;
  }
}
