"""Final HTTP response shapes; documentation types, not runtime dependencies.

Bodies contain exactly the declared required field (see openapi.yaml for
additionalProperties: false). MessageResponse describes the result after S5;
the scaffold retains the intermediate seed message until that implementation.
"""

from typing import Literal, TypedDict


class HealthResponse(TypedDict):
    status: Literal["ok"]


class MessageResponse(TypedDict):
    message: Literal["sandbox-release smoke"]


class NotFoundResponse(TypedDict):
    error: Literal["not_found"]
