// The Models page's data, as the server's modelsData gives it. Each tab fills only its own part.

// What every tab reads of the registry: how a provider may be added, and the models a person may be limited to.
export interface ModelsView {
  Kinds: string[] | null
  Available: string[] | null
  Private: boolean
  Editable: boolean
  Unnamed: boolean
}

export interface ModelsTab {
  Key: string
  Label: string
  Href: string
  On: boolean
}

export interface Pages {
  Shown: string
  Previous: string
  Next: string
}

// A price is dollars for a million tokens. A model with no price is not counted against a budget.
export interface Price {
  inputPerMillion: number
  outputPerMillion: number
  cachedInputPerMillion?: number
}

// One model the organization has, as the Models tab lists it, its prices already in words.
export interface ModelRow {
  Name: string
  Full: string
  Provider: string
  Input: string
  Output: string
  Who: string
  Off: boolean
}

// One provider as the Providers tab lists it.
export interface ProviderRow {
  Name: string
  Href: string
  Address: string
  Key: string
  State: string
  More: number
  Models: number
  Operator: boolean
}

export interface Address {
  Address: string
  Kind: string
  Extra: boolean
}

export interface ProviderModel {
  Name: string
  Full: string
  Price: Price | null
  Pending: boolean
}

// One provider, with where it answers and its key. A provider the operator set is shown and not changed.
export interface Provider {
  Name: string
  Addresses: Address[] | null
  Operator: boolean
  Pending: boolean
  Key: string
  Problem: string
}

export interface KeySource {
  Kind: string
  Words: string
  On: boolean
}

export interface Limit {
  email: string
  models: string[] | null
}

// An earlier state of the registry, with what was changed after it.
export interface Step {
  Version: number
  At: number
  Then: string[] | null
}

export interface DefaultChoice {
  Value: string
  Label: string
  Chosen: boolean
}

export interface RunTokens {
  Bound: boolean
  Err: string
}

export interface ModelsData {
  View: ModelsView
  Refused: string
  Tabs: ModelsTab[]
  Tab: string
  Pages?: Pages
  Rows?: ModelRow[] | null
  Providers?: ProviderRow[] | null
  Sources?: KeySource[]
  Provider?: Provider
  Models?: ProviderModel[] | null
  People?: string[] | null
  Limits?: Limit[] | null
  Steps?: Step[] | null
  Defaults?: DefaultChoice[] | null
  DefaultNow?: string
  RunTokens?: RunTokens
}
