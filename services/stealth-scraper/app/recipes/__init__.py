"""Provider resolution recipes for the stealth-scraper.

A recipe drives a real browser page through a provider's player chain and
returns a resolved stream session. Pure-logic helpers (URL/host/payload
parsing) live alongside the async recipe and are unit-tested directly.
"""

from .base import (
    ChallengeError,
    NotFoundError,
    Recipe,
    RecipeContext,
    RecipeError,
    host_allowed,
    looks_like_challenge,
)

__all__ = [
    "Recipe",
    "RecipeContext",
    "RecipeError",
    "ChallengeError",
    "NotFoundError",
    "host_allowed",
    "looks_like_challenge",
]
