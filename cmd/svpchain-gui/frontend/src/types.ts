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

export type UpdateInfo = {
  Current: string
  Latest: string
  TagName: string
  ReleaseURL: string
}
