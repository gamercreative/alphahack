"""Example Cornilia tools for the demo backend, and how a harness loop uses the guard."""
from __future__ import annotations

from typing import Annotated

from pydantic import Field

from cornilia_guard import Risk, StrictArgs, ToolGuard, ToolSpec


class SearchEvents(StrictArgs):
    query: Annotated[str, Field(max_length=100, description="words in the event title")] = ""
    city: Annotated[str | None, Field(max_length=40)] = None
    limit: Annotated[int, Field(ge=1, le=50)] = 10


class NoArgs(StrictArgs):
    pass


class CreateBooking(StrictArgs):
    event_id: Annotated[str, Field(pattern=r"^[a-z]{2,4}_[A-Za-z0-9]{1,32}$")]
    seats: Annotated[int, Field(ge=1, le=6)]


TOOLS = [
    ToolSpec("search_events", "events.search", SearchEvents, Risk.READ,
             "Search upcoming events by title words and optional city."),
    ToolSpec("list_my_bookings", "bookings.list_mine", NoArgs, Risk.READ,
             "List the current user's bookings."),
    ToolSpec("create_booking", "bookings.create", CreateBooking, Risk.WRITE,
             "Book seats on an event for the current user."),
]


def run_tool_calls(guard: ToolGuard, tool_calls: list[dict]) -> list[dict]:
    """Drop-in for Cornilia's tool step: turn the model's tool calls into tool messages."""
    out = []
    for tc in tool_calls:
        fn = tc["function"]
        res = guard.execute(fn["name"], fn.get("arguments"))
        out.append({"role": "tool", "tool_call_id": tc.get("id", ""), "content": res.content})
        if guard.should_stop:
            break
    return out
