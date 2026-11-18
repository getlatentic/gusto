import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Choices, Field } from "@/pages/admin/apps/field"
import type { ImportedChoice } from "@/pages/admin/apps/types"
import { withBase } from "@/lib/base"

// The forms that add a skill, each posted as the browser sends it; the server answers with the new skill set's or plugin's page.

const note = "m-0 text-sm text-muted-foreground"

// WriteSkillSetForm names a new skill set and chooses the tools and plugins it is given. It is off until turned on.
export function WriteSkillSetForm({
  servers,
  imported,
}: {
  servers: string[]
  imported: ImportedChoice[]
}) {
  return (
    <form
      className="grid max-w-xl gap-4"
      method="post"
      action={withBase("/admin/skill-sets")}
    >
      <Field id="add-name" label="Name">
        <Input
          id="add-name"
          name="name"
          placeholder="code-reviewer"
          pattern="[a-z0-9][a-z0-9\-]*"
          required
        />
      </Field>
      {servers.length > 0 && (
        <Choices
          legend="Tools"
          name="server"
          choices={servers.map((name) => ({ value: name, label: name }))}
        />
      )}
      {imported.length > 0 && (
        <Choices
          legend="Imported plugins"
          name="integration"
          choices={imported.map((plugin) => ({
            value: plugin.Name,
            label: <PluginName plugin={plugin} />,
          }))}
        />
      )}
      <p className={note}>It is off until you turn it on from its page.</p>
      <div>
        <Button type="submit">Create</Button>
      </div>
    </form>
  )
}

// PluginName is an imported plugin as a choice names it, saying when it is off.
export function PluginName({ plugin }: { plugin: ImportedChoice }) {
  return (
    <>
      {plugin.Name}
      {!plugin.On && <span className="text-muted-foreground"> off</span>}
    </>
  )
}

// GitImportForm imports an Agent Plugin from a repository, pinned to the commit its branch or tag resolves to.
export function GitImportForm() {
  return (
    <form
      className="grid max-w-xl gap-4"
      method="post"
      action={withBase("/admin/integrations/import")}
      encType="multipart/form-data"
    >
      <input type="hidden" name="kind" value="git" />
      <Field id="import-address" label="Git address">
        <Input
          id="import-address"
          name="address"
          placeholder="https://github.com/example/plugin.git"
          required
        />
      </Field>
      <Field
        id="import-ref"
        label="Branch, tag or commit"
        hint="· pinned to the commit it resolves to"
      >
        <Input id="import-ref" name="ref" placeholder="main" />
      </Field>
      <Field
        id="import-folder"
        label="Folder in the repository"
        hint="· only if the plugin is not at the top"
      >
        <Input id="import-folder" name="folder" placeholder="plugins/reports" />
      </Field>
      <p className={note}>
        An imported plugin is off until you turn a version on.
      </p>
      <div>
        <Button type="submit">Import from git</Button>
      </div>
    </form>
  )
}

// UploadImportForm imports an Agent Plugin from an archive sent with the form.
export function UploadImportForm() {
  return (
    <form
      className="grid max-w-xl gap-4"
      method="post"
      action={withBase("/admin/integrations/import")}
      encType="multipart/form-data"
    >
      <input type="hidden" name="kind" value="upload" />
      <Field id="import-archive" label="A .zip or .tar.gz of the plugin">
        <Input
          id="import-archive"
          type="file"
          name="archive"
          accept=".zip,.gz,.tgz"
          required
        />
      </Field>
      <p className={note}>
        An imported plugin is off until you turn a version on.
      </p>
      <div>
        <Button type="submit">Import the upload</Button>
      </div>
    </form>
  )
}
