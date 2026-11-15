import { LinkTabs, Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { usePageData } from "@/lib/api"
import { AccessTab } from "./access-tab"
import { AddProvider } from "./add-provider"
import { PricesTab } from "./prices-tab"
import { ProviderTab } from "./provider-tab"
import { ProvidersTab } from "./providers-tab"
import { SettingsTab } from "./settings-tab"
import type { ModelsData } from "./types"

// Where models answer, what they cost, and who may use them, a tab at a time. The Providers tab can name one provider, which
// is then the page.
export function ModelsPage() {
  const loaded = usePageData<ModelsData>()
  const page = loaded.state === "ready" ? loaded.data : undefined
  const provider = page?.Provider
  return (
    <>
      <PageBar
        title={provider ? provider.Name : "Models"}
        crumbs={
          provider
            ? [{ label: "Models", href: "/admin/models?tab=providers" }]
            : []
        }
      >
        {page && !page.View.Unnamed && <AddProvider view={page.View} />}
      </PageBar>
      <PageBody>
        <PageSub>
          Where models answer, what they cost, and who may use them
        </PageSub>
        <Loading loaded={loaded}>{(data) => <Models page={data} />}</Loading>
      </PageBody>
    </>
  )
}

function Models({ page }: { page: ModelsData }) {
  return (
    <>
      <LinkTabs
        label="Views"
        current={page.Tab}
        tabs={page.Tabs.map((tab) => ({
          key: tab.Key,
          label: tab.Label,
          href: tab.Href,
        }))}
      />
      {page.Refused && <Notice>{page.Refused}</Notice>}
      <ModelsTabBody page={page} />
    </>
  )
}

function ModelsTabBody({ page }: { page: ModelsData }) {
  switch (page.Tab) {
    case "models":
      return <PricesTab page={page} />
    case "providers":
      return page.Provider ? (
        <ProviderTab provider={page.Provider} page={page} />
      ) : (
        <ProvidersTab page={page} />
      )
    case "access":
      return <AccessTab page={page} />
    default:
      return <SettingsTab page={page} />
  }
}
