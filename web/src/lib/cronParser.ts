import type { TFunction } from "i18next";

export interface CronParseResult {
  valid: boolean;
  error?: string;
  description?: string;
}

function validateField(field: string, min: number, max: number, t: TFunction): string | null {
  if (field === "*") return null;
  for (const segment of field.split(",")) {
    const [rangePart, stepStr] = segment.split("/");
    if (stepStr !== undefined) {
      const step = Number(stepStr);
      if (!Number.isInteger(step) || step <= 0) return t("analytics.invalidStep", { value: stepStr });
    }
    if (rangePart === "*") continue;
    if (rangePart.includes("-")) {
      const [lo, hi] = rangePart.split("-").map(Number);
      if (!Number.isInteger(lo) || !Number.isInteger(hi)) return t("analytics.invalidRange", { value: rangePart });
      if (lo < min || hi > max || lo > hi) return t("analytics.rangeExceeded", { lo, hi, min, max });
    } else {
      const v = Number(rangePart);
      if (!Number.isInteger(v)) return t("analytics.invalidValue", { value: rangePart });
      if (v < min || v > max) return t("analytics.valueExceeded", { value: v, min, max });
    }
  }
  return null;
}

function getWeekdayNames(t: TFunction): string[] {
  return [
    t("analytics.weekdaySun"),
    t("analytics.weekdayMon"),
    t("analytics.weekdayTue"),
    t("analytics.weekdayWed"),
    t("analytics.weekdayThu"),
    t("analytics.weekdayFri"),
    t("analytics.weekdaySat"),
  ];
}

export function expandList(field: string): number[] {
  const result = new Set<number>();
  for (const segment of field.split(",")) {
    const [rangePart, stepStr] = segment.split("/");
    const step = stepStr ? Number(stepStr) : 1;
    if (rangePart.includes("-")) {
      const [lo, hi] = rangePart.split("-").map(Number);
      for (let i = lo; i <= hi; i += step) result.add(i);
    } else {
      result.add(Number(rangePart));
    }
  }
  return [...result].sort((a, b) => a - b);
}

function describeList(field: string, fmt: (v: number) => string, t: TFunction): string {
  if (field.startsWith("*/")) return t("analytics.cronEveryN", { n: field.slice(2) });
  const values = expandList(field);
  if (values.length <= 5) return values.map(fmt).join(", ");
  return t("analytics.cronRangeSummary", { from: fmt(values[0]), to: fmt(values[values.length - 1]), count: values.length });
}

function describeCron(minute: string, hour: string, day: string, month: string, weekday: string, t: TFunction): string {
  const parts: string[] = [];
  const weekdayNames = getWeekdayNames(t);

  if (month !== "*") {
    parts.push(describeList(month, (v) => t("analytics.cronMonthN", { n: v }), t));
  }
  if (day !== "*") {
    parts.push(describeList(day, (v) => t("analytics.cronDayN", { n: v }), t));
  }
  if (weekday !== "*") {
    if (weekday === "1-5") {
      parts.push(t("analytics.cronWeekdays"));
    } else if (weekday === "0,6") {
      parts.push(t("analytics.cronWeekends"));
    } else {
      parts.push(describeList(weekday, (v) => t("analytics.cronWeekdayN", { name: weekdayNames[v] ?? v }), t));
    }
  }

  if (hour === "*" && minute === "*") {
    parts.push(t("analytics.cronEveryMinute"));
  } else if (hour === "*") {
    if (minute.startsWith("*/")) {
      parts.push(t("analytics.cronEveryNMinutes", { n: minute.slice(2) }));
    } else if (minute === "0") {
      parts.push(t("analytics.cronEveryHourOnTheHour"));
    } else {
      parts.push(t("analytics.cronEveryHourAtMinute", { minute }));
    }
  } else if (minute === "*") {
    parts.push(describeList(hour, (v) => t("analytics.cronHourN", { n: v }), t) + t("analytics.cronEveryMinuteOf"));
  } else {
    if (hour.startsWith("*/")) {
      const m = minute === "0" ? t("analytics.cronOnTheHour") : t("analytics.cronAtMinute", { m: minute });
      parts.push(t("analytics.cronEveryNHours", { n: hour.slice(2), m }));
    } else {
      const hours = expandList(hour);
      const mins = minute === "0" ? "00" : minute.padStart(2, "0");
      if (hours.length <= 3) {
        parts.push(hours.map((h) => `${String(h).padStart(2, "0")}:${mins}`).join(", "));
      } else {
        parts.push(`${describeList(hour, (v) => t("analytics.cronHourN", { n: v }), t)} ${mins}${t("analytics.cronMinuteSuffix")}`);
      }
    }
  }

  return parts.join(" ") || t("analytics.cronEveryMinute");
}

export function parseCronExpr(expr: string, t: TFunction): CronParseResult {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) {
    return { valid: false, error: t("analytics.cronNeedsFields", { count: parts.length }) };
  }

  const fieldNames = [
    t("analytics.cronFieldMinute"),
    t("analytics.cronFieldHour"),
    t("analytics.cronFieldDay"),
    t("analytics.cronFieldMonth"),
    t("analytics.cronFieldWeekday"),
  ];
  const ranges: [number, number][] = [[0, 59], [0, 23], [1, 31], [1, 12], [0, 6]];

  for (let i = 0; i < 5; i++) {
    const err = validateField(parts[i], ranges[i][0], ranges[i][1], t);
    if (err) return { valid: false, error: `${fieldNames[i]}: ${err}` };
  }

  const desc = describeCron(parts[0], parts[1], parts[2], parts[3], parts[4], t);
  return { valid: true, description: desc };
}
