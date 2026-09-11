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

export type SettlementRefund = {
    chain_id: string
    payer: string
    intent_id: string
    display_intent_id?: string
    task_id?: string
    task_status: string
    amount?: string
    available: string
    token: string
    refundable: boolean
    cancellable: boolean
    validator_state?: string
    validator_error?: string
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
    reply?: string
    tool_calls?: {id?: string; name: string; args?: string}[]
}

export type AgentTxCheck = {
    hash: string
    status: string
    code?: number
    height?: string
    raw_log?: string
    error?: string
    checked_at?: string
}

export type AgentIntentCheck = {
    kind: string
    tool: string
    expect?: Record<string, string>
    status: string
    detail?: string
    observed?: Record<string, string>
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
    session_id?: string
    session_title?: string
    tx_hashes?: string[]
    tx_checks?: AgentTxCheck[]
    intent_checks?: AgentIntentCheck[]
    round_count: number
    usage?: {
        prompt_tokens?: number
        completion_tokens?: number
        total_tokens?: number
    }
    prompt_sha256?: string
    skills?: string[]
    llm_rounds?: AgentLLMRound[]
    steps: AgentRunStep[]
}
