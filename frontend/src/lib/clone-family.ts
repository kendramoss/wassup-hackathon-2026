// A cloned case carries its planner family at the top level; older or
// product-only records may carry it on the product instead. Never substitute
// the UI default here: an LED/LiDAR case would silently be planned with the
// motorised-tools template (#298).
export function clonedFamily(source: { family?: string; product?: { family?: string } }): string | undefined {
  return source.family || source.product?.family || undefined;
}
