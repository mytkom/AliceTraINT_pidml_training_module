# PDI V2 Integration - Setup & Execution Plan

This document outlines the steps to set up and run the migrated AliceTraINT training module. The system now utilizes the new PDI library (Attention architecture) and features a cross-platform Go runner.


## 1. Setup Instructions

### A. Clone the Repository
Ensure you clone with submodules to get the PDI source code:
```bash
git clone --recursive <repository-url>
```

### B. Compile the Go Module
```bash
go build -o training-module cmd/AliceTraINT_pidml_training_module/main.go
```

### C. Python Environment
Create a virtual environment and install dependencies.
```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

### D. Configuration (.env)
Create your local `.env` file from the example:
```bash
cp .env.example .env
```
**Crucial:** Set `ALICETRAINT_VENV_DIR_PATH` to the name of your venv folder (e.g., `.venv`). The Go runner automatically detects the OS and appends `bin/python3` (Unix) or `Scripts/python.exe` (Windows).

### E. Data Preparation
Place your training data in the `data/` directory. For best results with the current automated logic, name it:
`data/preprocessed_ao2ds.root`

## 2. Execution

### Step 1: Start the Mock Server
In the first terminal (ensure `flask` is installed):
```bash
python mock_server.py
```

### Step 2: Start the Training Module
In a second terminal:
```bash
./training-module
```