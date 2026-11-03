import { format, isValid, parseISO } from "date-fns";

/**
 * The access window is stored as timestamps; the form edits whole days in
 * the admin's local time zone (DatePicker values are `yyyy-MM-dd`).
 * Start = 00:00 local of the first day, end = 23:59:59.999 local of the last
 * day, so "last day" means the members can still use it all that day.
 */
export function dayToStartISO(day: string): string {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y!, m! - 1, d!, 0, 0, 0, 0).toISOString();
}

export function dayToEndISO(day: string): string {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y!, m! - 1, d!, 23, 59, 59, 999).toISOString();
}

/** Timestamp → `yyyy-MM-dd` in local time ("" when empty/invalid). */
export function isoToDay(value: string | null | undefined): string {
  if (!value) return "";
  const date = parseISO(value);
  return isValid(date) ? format(date, "yyyy-MM-dd") : "";
}
