"""Tool definitions: each Cornilia tool maps 1:1 to an allowlisted gateway op."""
from __future__ import annotations

from dataclasses import dataclass
from enum import IntEnum
from typing import Any

from pydantic import BaseModel, ConfigDict


class Risk(IntEnum):
    READ = 0         # no state change
    WRITE = 1        # changes state; approval rules apply
    DESTRUCTIVE = 2  # deletes / irreversible; always needs a human yes


class StrictArgs(BaseModel):
    """Base for tool args: unknown fields are an error, not silently dropped.
    This is where a model trying to pass `user_id` gets stopped first."""
    model_config = ConfigDict(extra="forbid", str_strip_whitespace=True)


@dataclass(frozen=True)
class ToolSpec:
    name: str                 # what the model calls, e.g. "search_events"
    op: str                   # gateway op, e.g. "events.search"
    args_model: type[StrictArgs]
    risk: Risk
    description: str

    def schema(self) -> dict[str, Any]:
        """Provider-agnostic function schema (OpenAI-style; Ollama accepts it too)."""
        params = self.args_model.model_json_schema()
        params.pop("title", None)
        return {"type": "function", "function": {
            "name": self.name, "description": self.description, "parameters": params}}
