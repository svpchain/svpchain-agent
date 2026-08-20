export type Entry = { ChainID: string; Owner: string; EVMAddr: string }

export type WhitelistEntry = {
    ChainID: string
    AddressType: string
    Address: string
    Alias: string
}

export type SkillSetting = {
    name: string
    description: string
    enabled: boolean
    locked: boolean
    source: string
}

export type DiscoveredAgent = {
    agent_id: string
    endpoint: string
    capabilities: string[]
    pricing_text: string
    bond_text: string
    metadata: string
    card_verified: boolean
    card_error?: string
}

export type DelegationRow = {
    root_id: string
    agent_id: string
    self_issued: boolean
    actions: string[]
    subaccounts: number[]
    total_cap: string
    daily_cap: string
    spent_total: string
    epoch: number
    paused: boolean
    expires_at: number
}

export type UpdateInfo = {
    Current: string
    Latest: string
    TagName: string
    ReleaseURL: string
}

export type AgentRunOutcome = 'success' | 'failed' | 'stopped' | 'rejected' | 'cancelled' | string

export type AgentRunStep = {
    at: string
    kind: string
    round?: number
    tool?: string
    args?: string
    ok?: boolean | null
    detail?: string
    result?: string
    elapsed_ms?: number
}

export type AgentLLMRound = {
    round: number
    latency_ms: number
    model?: string
    prompt_tokens?: number
    completion_tokens?: number
    total_tokens?: number
}

export type AgentRun = {
    run_id: string
    started_at: string
    finished_at?: string
    chain_id: string
    remote_url: string
    model?: string
    provider?: string
    user_message: string
    outcome: AgentRunOutcome
    answer?: string
    error?: string
    tx_hashes?: string[]
    round_count: number
    usage?: {
        prompt_tokens?: number
        completion_tokens?: number
        total_tokens?: number
    }
    llm_rounds?: AgentLLMRound[]
    steps: AgentRunStep[]
}
