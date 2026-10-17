// Plugins whose providers declare where their vendor answers: one that
// answers on three APIs and names each (the endpoints field, magpie's own,
// #2409), one that names none and answers at its api, and one that declares
// nothing at all.
export const Multi = async () => ({
  config: async (cfg) => {
    cfg.provider = cfg.provider ?? {}
    cfg.provider.relayco = {
      name: "RelayCo",
      npm: "@ai-sdk/openai-compatible",
      api: "https://relay.invalid/v1",
      endpoints: {
        responses: "https://relay.invalid/responses",
        anthropic: " https://relay.invalid/anthropic ",
        gemini: { url: "https://relay.invalid/v1beta" }, // not an address: dropped
        chat: "", // said empty: dropped
      },
      models: {
        "relay-chat": {},
        "relay-claude": { provider: { npm: "@ai-sdk/anthropic" } },
        "relay-gpt": { provider: { npm: "@ai-sdk/openai" } },
      },
    }
  },
  auth: { provider: "relayco", methods: [{ type: "api", label: "API key" }] },
})

// a provider whose plugin says where it is, and nothing about its APIs
export const AtAPI = async () => ({
  config: async (cfg) => {
    cfg.provider = cfg.provider ?? {}
    cfg.provider.oneline = {
      name: "One Line",
      npm: "@ai-sdk/openai-compatible",
      api: "https://oneline.invalid/v1",
      models: { "oneline-1": {} },
    }
  },
  auth: { provider: "oneline", methods: [{ type: "api", label: "API key" }] },
})

// a provider whose plugin declares neither
export const Silent = async () => ({
  auth: { provider: "silentco", methods: [{ type: "api", label: "API key" }] },
})
