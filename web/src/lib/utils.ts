export const cn = (...values: Array<string | false | null | undefined>) =>
  values.filter((value): value is string => typeof value === "string" && value.length > 0).join(" ");
