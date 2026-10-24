"""Power helper built on top of :mod:`calc`."""

from calc import mul


def power(a: float, n: int) -> float:
    """Return ``a`` raised to the non-negative integer power ``n``.

    Multiplication is done with :func:`calc.mul`. ``power(a, 0)`` is ``1``.
    Raises ``ValueError`` if ``n`` is negative.
    """
    if n < 0:
        raise ValueError("n must be a non-negative integer")
    result = 1
    for _ in range(n):
        result = mul(result, a)
    return result
