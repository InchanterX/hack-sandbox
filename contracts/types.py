"""Final HTTP response types; wire schemas are defined in openapi.yaml."""

from typing import Literal, TypedDict


class HealthResponse(TypedDict):
    status: Literal["ok"]


class MessageResponse(TypedDict):
    message: Literal["sandbox-repair smoke"]


class ErrorResponse(TypedDict):
    error: Literal["not_found"]


type ResponseBody = HealthResponse | MessageResponse | ErrorResponse
