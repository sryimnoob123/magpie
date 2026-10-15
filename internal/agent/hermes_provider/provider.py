"""magpie's provider profile for Hermes Agent: what each model magpie serves accepts.

Hermes decides whether an image reaches the model natively — or is described to it by an
auxiliary model first (``agent/image_routing.py``) — from capability probes that read a
models.dev catalog entry, a managed local runtime's ``/props`` or Ollama's ``/api/show``,
never a provider's own ``GET /v1/models``. A gateway's models are in no catalog, so
``provider: magpie`` resolved to "unknown", and every image took the describe path: an extra
model call, and only its text reached the conversation.

This plugin registers the declaration Hermes looks for. ``models.json`` beside it is what
magpie wrote when it connected Hermes — the same entries ``GET /v1/models`` answers, so the
declaration never drifts from the gateway. When that file is missing (an older connect, or a
hand-installed copy) the gateway is asked once, and when it cannot be reached nothing is
declared and Hermes behaves exactly as it did before.

Written and refreshed by magpie (internal/agent/hermes_provider.go); safe to delete.
"""

from __future__ import annotations

import dataclasses
import importlib.util
import json
import sys
from pathlib import Path
from typing import Any

from providers import register_provider
from providers.base import ProviderProfile

NAME = "magpie"
ALIASES = ("magpie", "magpie-gateway", "magpie_gateway")
DEFAULT_BASE_URL = "http://127.0.0.1:3425/v1"
TIMEOUT_SECONDS = 1.5
MODELS_FILE = "models.json"


def _config_base_url() -> str:
    """magpie's base URL as Hermes's config has it, or "".

    Read straight from ``config.yaml`` with the shared YAML policy: this runs inside provider
    discovery, where importing ``hermes_cli`` is deliberately avoided.
    """
    try:
        import hermes_yaml
        from hermes_constants import get_hermes_home

        config = hermes_yaml.safe_load((Path(get_hermes_home()) / "config.yaml").read_text("utf-8"))
    except Exception:
        return ""
    if not isinstance(config, dict):
        return ""
    model = config.get("model") if isinstance(config.get("model"), dict) else {}
    providers = config.get("providers") if isinstance(config.get("providers"), dict) else {}
    for holder in (providers.get(NAME), {**model, "name": model.get("provider")}):
        if isinstance(holder, dict) and str(holder.get("name") or "").strip().lower() in ("", NAME):
            url = str(holder.get("base_url") or "").strip()
            if url:
                return url
    for entry in config.get("custom_providers") or []:
        if isinstance(entry, dict) and str(entry.get("name") or "").strip().lower() == NAME:
            url = str(entry.get("base_url") or "").strip()
            if url:
                return url
    return ""


def _declared_models() -> list[dict[str, Any]]:
    """magpie's model list: the file it wrote, else one live ``GET /v1/models``."""
    sidecar = Path(__file__).with_name(MODELS_FILE)
    if sidecar.is_file():
        try:
            payload = json.loads(sidecar.read_text("utf-8"))
            return list(payload.get("data") or []) if isinstance(payload, dict) else list(payload or [])
        except Exception:
            pass
    try:
        import httpx

        with httpx.Client(timeout=TIMEOUT_SECONDS, trust_env=False) as client:
            response = client.get((_config_base_url() or DEFAULT_BASE_URL).rstrip("/") + "/models")
            response.raise_for_status()
            return list((response.json() or {}).get("data") or [])
    except Exception:
        return []


def _capabilities(models: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    """Hermes's canonical capability declaration for magpie's model entries.

    ``agent/models_dev.py`` merges these names over the catalog entry: ``supports_vision``
    (patches ``modalities.input``), ``context_window``, ``supports_reasoning`` and
    ``supports_tools``.
    """
    declared: dict[str, dict[str, Any]] = {}
    for entry in models:
        if not isinstance(entry, dict):
            continue
        model = str(entry.get("id") or "").strip()
        if not model:
            continue
        capabilities: dict[str, Any] = {}
        modalities = entry.get("modalities") if isinstance(entry.get("modalities"), dict) else {}
        inputs = modalities.get("input")
        if isinstance(inputs, list) and inputs:
            capabilities["supports_vision"] = "image" in inputs
        context = entry.get("context_window", entry.get("context_length"))
        if isinstance(context, int) and context > 0:
            capabilities["context_window"] = context
        if isinstance(entry.get("reasoning"), bool):
            capabilities["supports_reasoning"] = entry["reasoning"]
        # A magpie endpoint is an agent-facing gateway: tool calls are its whole purpose.
        # Stated because an unspecified ``tool_call`` reads as "this model cannot call tools".
        capabilities["supports_tools"] = True
        declared[model] = capabilities
    return declared


def _bundled_custom() -> Any:
    """The bundled ``custom`` profile's module, or None.

    Discovery imports the bundled providers before any ``$HERMES_HOME`` one, so the module is
    already in ``sys.modules``; the file fallback covers a differently-laid-out install.
    """
    module = sys.modules.get("plugins.model_providers.custom")
    if getattr(module, "custom", None) is not None:
        return module
    try:
        from providers import base as providers_base

        root = Path(providers_base.__file__).resolve().parent.parent
        path = root / "plugins" / "model-providers" / "custom" / "__init__.py"
        if path.is_file():
            spec = importlib.util.spec_from_file_location(
                "_magpie_bundled_custom", path, submodule_search_locations=[str(path.parent)]
            )
            if spec and spec.loader:
                module = importlib.util.module_from_spec(spec)
                spec.loader.exec_module(module)
                if getattr(module, "custom", None) is not None:
                    return module
    except Exception:
        pass
    return None


def _profile_base() -> type[ProviderProfile]:
    """``CustomProfile`` when it is reachable, else the plain profile.

    magpie is an OpenAI-compatible custom endpoint, so the custom profile's behaviour (its
    wire effort clamp, ``num_ctx`` on an Ollama-shaped host, the medium reasoning default)
    must not be lost by registering a name of our own.
    """
    module = _bundled_custom()
    found = getattr(module, "CustomProfile", None)
    return found if isinstance(found, type) else ProviderProfile


class MagpieProfile(_profile_base()):
    """magpie: custom-endpoint behaviour plus the gateway's own model declaration."""


def _declare_on_custom(declared: dict[str, dict[str, Any]]) -> None:
    """Put the same declaration on the ``custom`` profile.

    Hermes canonicalizes a *named* custom provider to ``provider="custom"`` before the probes
    run and hands them only that name, so a profile under our own name is invisible where
    image routing looks. Only magpie's model ids are added, merged over what the bundled
    profile declares, so an unrelated custom endpoint is unaffected.
    """
    if not declared:
        return
    module = _bundled_custom()
    base = getattr(module, "custom", None)
    if base is None:
        return
    try:
        register_provider(
            dataclasses.replace(base, model_capabilities={**dict(base.model_capabilities), **declared})
        )
    except Exception:
        pass


_declared = _capabilities(_declared_models())

register_provider(
    MagpieProfile(
        name=NAME,
        aliases=ALIASES,
        display_name="magpie",
        description="Local model gateway — several providers and routing groups behind one OpenAI-compatible endpoint",
        base_url="",
        env_vars=(),
        # A magpie endpoint takes image parts in tool-result messages; whether a given model
        # *sees* an image is declared per model above, from the gateway's own report.
        supports_vision=True,
        model_capabilities=_declared,
    )
)
_declare_on_custom(_declared)
