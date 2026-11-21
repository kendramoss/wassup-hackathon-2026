// The table's source cell: the first committed file named, by its file name,
// with "+ N more" for the others; the full string is the cell's title.
// A row with no committed source says so.
export function sourceLabel(source?: string): string {
  if (!source) return "";
  if (source.startsWith("unavailable:")) return "not in this repository";
  const files = [...new Set(source.match(/[\w./-]+\.(?:jsonl|json|md|py|go)\b/g) ?? [])];
  if (files.length === 0) return source;
  const name = files[0].split("/").pop() ?? files[0];
  return files.length > 1 ? `${name} + ${files.length - 1} more` : name;
}
