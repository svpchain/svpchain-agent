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
