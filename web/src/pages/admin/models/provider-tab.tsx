import { Notice } from "@/components/page"
import { PostButton } from "./post-button"
import {
  AddAddress,
  AddModel,
  providerAction,
  ReplaceKey,
} from "./provider-dialogs"
import { ProviderModels } from "./provider-models"
import { StateBadge } from "./state-badge"
import type { ModelsData, Provider } from "./types"

// ProviderTab is one provider: where it answers, its key, and its models with their prices.
export function ProviderTab({
  provider,
  page,
}: {
  provider: Provider
  page: ModelsData
}) {
  return (
    <>
      <div className="mb-4 grid gap-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="m-0 flex flex-wrap items-center gap-2 text-base font-semibold">
            {provider.Name}
            {provider.Operator && (
              <span className="text-sm font-normal text-muted-foreground">
                set by the operator of this gusto
              </span>
            )}
            {provider.Pending && (
              <StateBadge tone="warn">
                off: waiting for you to turn it on
              </StateBadge>
            )}
          </h2>
          {!provider.Operator && <ProviderActions provider={provider} />}
        </div>
        <Addresses provider={provider} />
      </div>
      {provider.Problem && (
        <Notice tone="warn">Not in use: {provider.Problem}</Notice>
      )}
      <ProviderModels provider={provider} page={page} />
    </>
  )
}

function ProviderActions({ provider }: { provider: Provider }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <AddModel provider={provider} />
      <AddAddress provider={provider} />
      <PostButton
        action={providerAction(provider, "/discover")}
        variant="outline"
        size="sm"
        title={`Asks ${provider.Name} which models its key can use and adds each, with its price when it gives one, off until you turn it on`}
      >
        Find its models
      </PostButton>
      {provider.Key === "stored" && <ReplaceKey provider={provider} />}
      {provider.Pending ? (
        <PostButton
          action={providerAction(provider, "/enable")}
          variant="outline"
          size="sm"
        >
          Turn on {provider.Name}
        </PostButton>
      ) : (
        <PostButton
          action={providerAction(provider, "/disable")}
          variant="outline"
          size="sm"
        >
          Turn off {provider.Name}
        </PostButton>
      )}
      <PostButton
        action={providerAction(provider, "/retire")}
        variant="destructive"
        size="sm"
      >
        Retire {provider.Name}
      </PostButton>
    </div>
  )
}

// Addresses are where the provider answers, under its one key. One added beside the first can be taken away.
function Addresses({ provider }: { provider: Provider }) {
  const addresses = provider.Addresses ?? []
  return (
    <>
      <ul className="m-0 grid list-none gap-1 p-0 text-sm">
        {addresses.map((address) => (
          <li
            key={address.Address}
            className="flex flex-wrap items-center gap-2"
          >
            <code className="font-mono text-xs break-all">
              {address.Address}
            </code>
            <span className="text-muted-foreground">{address.Kind}</span>
            {address.Extra && !provider.Operator && (
              <PostButton
                action={providerAction(provider, "/addresses/remove")}
                fields={{ address: address.Address }}
                variant="destructive"
                size="xs"
                aria-label={`Remove ${address.Address}`}
              >
                Remove
              </PostButton>
            )}
          </li>
        ))}
      </ul>
      <p className="m-0 text-xs text-muted-foreground">
        Key: {provider.Key === "stored" ? "kept here" : provider.Key}
        {addresses.length > 1 &&
          ", the same at each address. A call goes to the address that answers the agent's kind of API."}
      </p>
    </>
  )
}
