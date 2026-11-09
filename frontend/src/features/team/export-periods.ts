export type SplitMode = "none" | "weekly" | "dates";
export type ExportPeriod = { from: string; to: string };
export type PeriodOptions = {
  from: string;
  to: string;
  split: SplitMode;
  boundary: "start" | "end";
  weekdays: number[];
  dates: string[];
};
function parse(value: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value))
    throw Error("Укажите начало и конец периода");
  const d = new Date(value + "T00:00:00Z");
  if (
    !Number.isFinite(d.getTime()) ||
    d.toISOString().slice(0, 10) !== value ||
    d.getUTCFullYear() < 1900
  )
    throw Error("Укажите корректные даты");
  return d;
}
const day = 86400000;
const key = (v: number) => new Date(v).toISOString().slice(0, 10);
export function exportPeriods(options: PeriodOptions): ExportPeriod[] {
  const first = parse(options.from).getTime(),
    last = parse(options.to).getTime();
  if (last < first || last - first > 365 * day)
    throw Error("Выберите период от 1 до 366 дней");
  const bounds = new Set([first, last + day]);
  const add = (stamp: number) => {
    const boundary = stamp + (options.boundary === "end" ? day : 0);
    if (boundary > first && boundary <= last) bounds.add(boundary);
  };
  if (options.split === "weekly") {
    if (!options.weekdays.length)
      throw Error("Выберите хотя бы один день недели");
    for (let stamp = first; stamp <= last; stamp += day)
      if (
        options.weekdays.includes(((new Date(stamp).getUTCDay() + 6) % 7) + 1)
      )
        add(stamp);
  } else if (options.split === "dates") {
    if (!options.dates.length) throw Error("Добавьте контрольную дату");
    for (const value of options.dates) {
      const stamp = parse(value).getTime();
      if (stamp < first || stamp > last)
        throw Error("Контрольные даты должны находиться внутри общего периода");
      add(stamp);
    }
  }
  const sorted = [...bounds].sort((a, b) => a - b);
  return sorted
    .slice(0, -1)
    .map((value, i) => ({ from: key(value), to: key(sorted[i + 1] - day) }));
}
