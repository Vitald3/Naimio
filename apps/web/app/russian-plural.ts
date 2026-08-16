export function russianPlural(count: number, one: string, few: string, many: string): string {
  const absolute = Math.abs(count) % 100;
  const last = absolute % 10;
  if (absolute > 10 && absolute < 20) return many;
  if (last === 1) return one;
  if (last >= 2 && last <= 4) return few;
  return many;
}

export function countLabel(count: number, forms: readonly [string, string, string]): string {
  return `${count} ${russianPlural(count, forms[0], forms[1], forms[2])}`;
}
