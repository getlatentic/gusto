// Instructions are a skill's body as the server drew it from Markdown: it draws HTML that was written as text, so none of it runs.
export function Instructions({ html }: { html: string }) {
  return (
    <div
      className="mt-1 max-w-[72ch] text-sm break-words [&_a]:text-primary [&_a]:underline [&_code]:rounded-xs [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_h1,&_h2,&_h3,&_h4]:mt-4 [&_h1,&_h2,&_h3,&_h4]:mb-2 [&_h1,&_h2,&_h3,&_h4]:font-semibold [&_li]:my-1 [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-2 [&_pre]:overflow-x-auto [&_pre]:rounded-sm [&_pre]:bg-muted [&_pre]:p-3 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_table]:block [&_table]:max-w-full [&_table]:overflow-x-auto [&_ul]:list-disc [&_ul]:pl-5"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
