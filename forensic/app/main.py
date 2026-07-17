from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from app.rag_engine import ForensicBrain

app = FastAPI(title="Metadata Crash Cart - Forensic Brain")
brain = ForensicBrain()

class CrashPayload(BaseModel):
    timestamp: str
    hex_dump: str
    alert_type: str

@app.post("/analyze")
async def analyze_crash(payload: CrashPayload):
    try:
        rca_result = brain.analyze_dump(payload.hex_dump)
        print(f"[Brain] Received crash alert ({payload.alert_type}).")
        print(f"[Brain] Analysis: {rca_result}")
        return {"status": "SUCCESS", "rca": rca_result}
    except Exception as e:
        print(f"[Brain] Error processing payload: {e}")
        raise HTTPException(status_code=500, detail=str(e))