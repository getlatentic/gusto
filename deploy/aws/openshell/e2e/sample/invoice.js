// Works out an invoice's total. Invented for demonstration.

// total is the sum of each line's price times its quantity, less the
// discount, plus tax on what remains.
export function total(lines, { discountPercent = 0, taxPercent = 0 } = {}) {
  let subtotal = 0;
  for (const line of lines) {
    subtotal += line.price * line.quantity;
  }
  const discount = subtotal * discountPercent;
  const taxed = (subtotal - discount) * (1 + taxPercent / 100);
  return Math.round(taxed * 100) / 100;
}
