import { test } from "node:test";
import assert from "node:assert/strict";
import { total } from "./invoice.js";

test("adds up the lines", () => {
  assert.equal(total([{ price: 10, quantity: 2 }, { price: 5, quantity: 1 }]), 25);
});

test("takes a percentage discount", () => {
  assert.equal(total([{ price: 100, quantity: 1 }], { discountPercent: 10 }), 90);
});

test("adds tax after the discount", () => {
  assert.equal(total([{ price: 100, quantity: 1 }], { discountPercent: 10, taxPercent: 20 }), 108);
});
