from fastapi import FastAPI
from pydantic import BaseModel
# Import the newly structured RAG engine
import app.rag_engine as rag_engine

app = FastAPI()

# 1. This Pydantic model now perfectly matches the Go Spy's JSON payload
class AlertPayload(BaseModel):
    timestamp: str
    hex_dump: str
    alert_type: str
    context: str

@app.post("/analyze")
async def analyze_telemetry(payload: AlertPayload):
    print(f"\n==================================================")
    print(f"[FastAPI] Incoming {payload.alert_type} Alert!")
    print(f"[FastAPI] Hex Dump: {payload.hex_dump}")
    print(f"[FastAPI] Hydrated Context: {payload.context}")
    
    # 2. We pass the English context string to the new AI function, NOT the raw hex
    print("[FastAPI] Querying ChromaDB Vector Store...")
    rca_result = rag_engine.analyze_error(payload.context)
    
    print(f"[FastAPI] Analysis Complete:")
    print(f" {rca_result}")
    print(f"==================================================\n")
    
    return {"status": "success", "analysis": rca_result}