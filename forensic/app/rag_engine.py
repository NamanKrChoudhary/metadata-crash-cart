from langchain_community.vectorstores import Chroma
from langchain_community.embeddings import HuggingFaceEmbeddings
from langchain.docstore.document import Document

print("[Brain] Initializing local HuggingFace embedding model...")
# Using the lightweight local model to keep everything on your silicon
embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")

print("[Brain] Ingesting Jira and Slack history into ChromaDB...")

# --- THE NEW SRE KNOWLEDGE BASE ---
# These mock tickets contain the exact semantic phrasing our Go Spy generates, 
# paired with the Root Cause Analysis (RCA) and Resolution steps.
mock_tickets = [
    Document(
        page_content="JIRA-801 [DYING BREATH]: The C++ engine executed a dying breath OS signal handler before termination. Captured code: 0xDEAD000B. ROOT CAUSE: Segmentation Fault (SIGSEGV) in the matching engine. RESOLUTION: Check for null pointer dereferences in the hot path. Restart engine immediately.",
        metadata={"source": "Jira", "ticket": "801"}
    ),
    Document(
        page_content="SLACK-ALERTS [SUDDEN DEATH]: Heartbeat lost. The C++ engine was instantly destroyed by the OS without a death rattle. ROOT CAUSE: Linux Out-Of-Memory (OOM) Killer terminated the process via SIGKILL. RESOLUTION: Engine exceeded RAM limits. Increase container swap space and memory allocation.",
        metadata={"source": "Slack", "alert": "OOM_KILL"}
    ),
    Document(
        page_content="JIRA-802 [LAPPING/RACE CONDITION]: The C++ engine lapped the observer. Dropped trades. Expected SeqID but found higher. Resyncing... ROOT CAUSE: CPU Starvation on the Observer core. C++ is writing faster than Go can read. RESOLUTION: Ensure Go Observer is strictly pinned to an isolated CPU core (taskset) and not context switching.",
        metadata={"source": "Jira", "ticket": "802"}
    ),
    Document(
        page_content="JIRA-803 [CORRUPTION]: The C++ trading engine encountered fatal memory corruption. Terminal exit code. ROOT CAUSE: Memory barrier failure (missing std::memory_order_release) or physical RAM bit-flip. RESOLUTION: Run memtest86 on physical server hardware.",
        metadata={"source": "Jira", "ticket": "803"}
    )
]

# Create the ephemeral vector store in memory
vectorstore = Chroma.from_documents(mock_tickets, embeddings)

def analyze_error(context_msg: str) -> str:
    """
    Takes the hydrated English string from the Go Spy, converts it to vectors, 
    and searches the Jira/Slack database for the closest historical match.
    """
    # k=1 means we only want the single most relevant historical ticket
    results = vectorstore.similarity_search_with_score(context_msg, k=1)
    
    if not results:
        return "[NOVEL ERROR] No historical context found in database. Escalate to human SRE."

    best_match, distance_score = results[0]

    # --- THE AI CONFIDENCE CUTOFF ---
    # ChromaDB uses L2 distance for this model. A lower score means a closer match.
    # If the mathematical distance is > 1.2, it means the Vector DB is guessing/hallucinating.
    if distance_score > 1.2:
        return f"[NOVEL ERROR] Signature is unknown (Distance: {distance_score:.2f}). Escalate to human SRE."

    # If the score is strong, return the historical fix!
    return f"[ROOT CAUSE FOUND] Match Score: {distance_score:.2f} | Action Plan: {best_match.page_content}"