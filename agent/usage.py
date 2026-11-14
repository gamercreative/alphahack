"""Token usage per run.

Cornilia v0.1.0 drops response.usage, so we wrap the provider instance's
OneShotMessage. The wrapper adds up the usage of every response and returns
the response unchanged. Cornilia itself is not edited.
"""


class UsageMeter:
    def __init__(self):
        self.input_tokens = 0
        self.output_tokens = 0
        self.calls = 0

    def add(self, response) -> None:
        usage = getattr(response, "usage", None)
        self.input_tokens += getattr(usage, "prompt_tokens", 0) or 0
        self.output_tokens += getattr(usage, "completion_tokens", 0) or 0
        self.calls += 1


def attach(provider) -> UsageMeter:
    """Wrap provider.OneShotMessage on this instance and return the meter that counts it."""
    meter = UsageMeter()
    original = provider.OneShotMessage

    def counted_one_shot(*args, **kwargs):
        response = original(*args, **kwargs)
        meter.add(response)
        return response

    provider.OneShotMessage = counted_one_shot
    return meter
