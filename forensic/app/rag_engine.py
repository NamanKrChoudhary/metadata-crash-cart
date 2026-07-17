from langchain.vectorstores import Chroma
from langchain.embeddings import HuggingFaceEmbeddings
from langchain.schema import Document

# 1. Emulating the "Dark History" (Jira & Slack Data)
MOCK_DARK_HISTORY = [
    Document(
        page_content="[Slack - #sre-alerts] @oncall System halted. Error signature 0xDEADBEEF caught in /dev/shm. Looks like the C++ producer lapped the Go observer during the Death Test. Consumer thread was starved.",
        metadata={"source": "Slack", "severity": "CRITICAL", "author": "devops_lead"}
    ),
    Document(
        page_content="[JIRA - ENG-402] Out of order execution detected on Core 2. Magic number corrupted to 0xBEEFCAFE. Root cause: Missing std::memory_order_release semantics on the ring buffer write_index.",
        metadata={"source": "Jira", "severity": "HIGH", "author": "hft_architect"}
    ),
    Document(
        page_content="[JIRA - ENG-311] Go worker pool flooded with false positives. Ghost loop detected because localReadIndex was not jumping forward after being lapped by the ring buffer.",
        metadata={"source": "Jira", "severity": "MEDIUM", "author": "qa_engineer"}
    )
]

class ForensicBrain:
    def __init__(self):
        # 2. Local, 100% Free Vector Embeddings (Runs on your hardware)
        print("[Brain] Initializing local HuggingFace embedding model...")
        self.embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")
        
        # 3. Load the Dark History into the Ephemeral Vector DB
        print("[Brain] Ingesting Jira and Slack history into ChromaDB...")
        self.vector_store = Chroma.from_documents(MOCK_DARK_HISTORY, self.embeddings)

    def analyze_dump(self, hex_dump: str) -> str:
        # 4. Ask for the top match AND its mathematical distance score
        results = self.vector_store.similarity_search_with_score(hex_dump, k=1)
        
        if not results:
            return "No historical matches found."
            
        best_match, distance_score = results[0]
        
        # 5. The Threshold Check (If distance is too high, reject the match as a hallucination)
        if distance_score > 1.2:
             return f"[NOVEL ERROR] Signature {hex_dump} is unknown. Distance score ({distance_score:.2f}) is too high. Escalate to human SRE."
        
        source = best_match.metadata['source']
        severity = best_match.metadata['severity']
        
        return f"[{severity} ALERT] Match Score: {distance_score:.2f} | Context via {source}: {best_match.page_content}"