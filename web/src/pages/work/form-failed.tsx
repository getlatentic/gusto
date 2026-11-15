// FormFailed says why the server turned a form away outright, when it did.
export function FormFailed({ failed }: { failed: string }) {
  if (!failed) return null
  return (
    <p
      role="alert"
      className="m-0 mt-3 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
    >
      {failed}
    </p>
  )
}
