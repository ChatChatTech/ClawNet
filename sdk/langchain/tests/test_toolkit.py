"""Tests for ClawNet LangChain toolkit and retriever."""

import json
from unittest.mock import patch, MagicMock

import pytest
from langchain_core.tools import BaseTool
from langchain_core.documents import Document

from clawnet_langchain import (
    ClawNetToolkit,
    ClawNetKnowledgeSearchTool,
    ClawNetTaskCreateTool,
    ClawNetReputationTool,
    ClawNetDiscoverTool,
    ClawNetStatusTool,
    ClawNetTopicSendTool,
    ClawNetRetriever,
)


# ── Tool instantiation ──

class TestToolInstantiation:
    def test_knowledge_search_tool(self):
        tool = ClawNetKnowledgeSearchTool()
        assert tool.name == "clawnet_knowledge_search"
        assert isinstance(tool, BaseTool)

    def test_task_create_tool(self):
        tool = ClawNetTaskCreateTool()
        assert tool.name == "clawnet_task_create"
        assert isinstance(tool, BaseTool)

    def test_reputation_tool(self):
        tool = ClawNetReputationTool()
        assert tool.name == "clawnet_reputation"
        assert isinstance(tool, BaseTool)

    def test_discover_tool(self):
        tool = ClawNetDiscoverTool()
        assert tool.name == "clawnet_discover"
        assert isinstance(tool, BaseTool)

    def test_status_tool(self):
        tool = ClawNetStatusTool()
        assert tool.name == "clawnet_status"
        assert isinstance(tool, BaseTool)

    def test_topic_send_tool(self):
        tool = ClawNetTopicSendTool()
        assert tool.name == "clawnet_topic_send"
        assert isinstance(tool, BaseTool)


# ── Toolkit ──

class TestToolkit:
    def test_get_tools_returns_all(self):
        toolkit = ClawNetToolkit()
        tools = toolkit.get_tools()
        assert len(tools) == 6
        assert all(isinstance(t, BaseTool) for t in tools)

    def test_get_tools_names(self):
        toolkit = ClawNetToolkit()
        names = {t.name for t in toolkit.get_tools()}
        expected = {
            "clawnet_knowledge_search",
            "clawnet_task_create",
            "clawnet_reputation",
            "clawnet_discover",
            "clawnet_status",
            "clawnet_topic_send",
        }
        assert names == expected

    def test_custom_base_url(self):
        toolkit = ClawNetToolkit(base_url="http://myhost:9999")
        tools = toolkit.get_tools()
        for t in tools:
            assert t.base_url == "http://myhost:9999"


# ── Mocked HTTP calls ──

def _mock_response(data, status_code=200):
    resp = MagicMock()
    resp.status_code = status_code
    resp.json.return_value = data
    resp.raise_for_status.return_value = None
    return resp


class TestToolCalls:
    @patch("clawnet_langchain.toolkit.httpx.Client")
    def test_knowledge_search(self, mock_client_cls):
        payload = [{"id": "k1", "title": "Python tips", "body": "Use asyncio"}]
        ctx = MagicMock()
        ctx.__enter__ = MagicMock(return_value=ctx)
        ctx.__exit__ = MagicMock(return_value=False)
        ctx.get.return_value = _mock_response(payload)
        mock_client_cls.return_value = ctx

        tool = ClawNetKnowledgeSearchTool()
        result = tool._run(query="python", limit=5)
        parsed = json.loads(result)
        assert len(parsed) == 1
        assert parsed[0]["title"] == "Python tips"

    @patch("clawnet_langchain.toolkit.httpx.Client")
    def test_knowledge_search_empty(self, mock_client_cls):
        ctx = MagicMock()
        ctx.__enter__ = MagicMock(return_value=ctx)
        ctx.__exit__ = MagicMock(return_value=False)
        ctx.get.return_value = _mock_response([])
        mock_client_cls.return_value = ctx

        tool = ClawNetKnowledgeSearchTool()
        result = tool._run(query="nonexistent")
        assert result == "No knowledge entries found."

    @patch("clawnet_langchain.toolkit.httpx.Client")
    def test_task_create(self, mock_client_cls):
        payload = {"id": "t1", "title": "Test task", "status": "open"}
        ctx = MagicMock()
        ctx.__enter__ = MagicMock(return_value=ctx)
        ctx.__exit__ = MagicMock(return_value=False)
        ctx.post.return_value = _mock_response(payload)
        mock_client_cls.return_value = ctx

        tool = ClawNetTaskCreateTool()
        result = tool._run(title="Test task", reward=50, tags="ai,ml")
        parsed = json.loads(result)
        assert parsed["title"] == "Test task"
        call_args = ctx.post.call_args
        body = call_args[1]["json"] if "json" in call_args[1] else call_args[0][1]
        assert body["tags"] == ["ai", "ml"]

    @patch("clawnet_langchain.toolkit.httpx.Client")
    def test_status(self, mock_client_cls):
        payload = {"peers": 42, "version": "0.8.0"}
        ctx = MagicMock()
        ctx.__enter__ = MagicMock(return_value=ctx)
        ctx.__exit__ = MagicMock(return_value=False)
        ctx.get.return_value = _mock_response(payload)
        mock_client_cls.return_value = ctx

        tool = ClawNetStatusTool()
        result = tool._run()
        parsed = json.loads(result)
        assert parsed["peers"] == 42


# ── Retriever ──

class TestRetriever:
    def test_instantiation(self):
        r = ClawNetRetriever()
        assert r.base_url == "http://localhost:3998"
        assert r.max_results == 10

    @patch("clawnet_langchain.retriever.httpx.Client")
    def test_get_relevant_documents(self, mock_client_cls):
        entries = [
            {
                "id": "k1",
                "title": "Async Python",
                "body": "Use asyncio for concurrency",
                "author_id": "peer1",
                "author_name": "Alice",
                "domains": ["python"],
                "type": "article",
                "created_at": "2025-01-01T00:00:00Z",
            }
        ]
        ctx = MagicMock()
        ctx.__enter__ = MagicMock(return_value=ctx)
        ctx.__exit__ = MagicMock(return_value=False)
        ctx.get.return_value = _mock_response(entries)
        mock_client_cls.return_value = ctx

        retriever = ClawNetRetriever()
        docs = retriever._get_relevant_documents("async python")
        assert len(docs) == 1
        assert isinstance(docs[0], Document)
        assert "Async Python" in docs[0].page_content
        assert docs[0].metadata["author_name"] == "Alice"
