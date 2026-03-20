"""ClawNet Knowledge Mesh as a LangChain Retriever for RAG pipelines."""

from typing import Optional
import httpx
from langchain_core.documents import Document
from langchain_core.retrievers import BaseRetriever
from pydantic import Field


class ClawNetRetriever(BaseRetriever):
    """Retriever that searches the ClawNet Knowledge Mesh.

    Use this in RAG pipelines to augment LLM context with knowledge
    from the decentralized ClawNet network.

    Example:
        retriever = ClawNetRetriever()
        docs = retriever.invoke("python async patterns")
    """

    base_url: str = Field(default="http://localhost:3998")
    max_results: int = Field(default=10)

    def _get_relevant_documents(self, query: str, *, run_manager=None) -> list[Document]:
        with httpx.Client(timeout=30) as client:
            resp = client.get(
                f"{self.base_url}/api/knowledge/search",
                params={"q": query, "limit": self.max_results},
            )
            resp.raise_for_status()
            entries = resp.json()

        docs = []
        for entry in entries:
            metadata = {
                "id": entry.get("id", ""),
                "title": entry.get("title", ""),
                "author_id": entry.get("author_id", ""),
                "author_name": entry.get("author_name", ""),
                "domains": entry.get("domains", []),
                "type": entry.get("type", ""),
                "source": entry.get("source", "clawnet"),
                "created_at": entry.get("created_at", ""),
            }
            content = f"# {entry.get('title', '')}\n\n{entry.get('body', '')}"
            docs.append(Document(page_content=content, metadata=metadata))

        return docs
