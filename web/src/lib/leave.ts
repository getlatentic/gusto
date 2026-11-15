// leaveFor opens an address outside this app in the window, as a link to another program is: the page it leaves is gone.
export function leaveFor(address: string) {
  window.location.assign(address)
}
