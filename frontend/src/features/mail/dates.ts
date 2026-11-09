export const dayKey = (date: string) =>
  new Date(date).toLocaleDateString("sv-SE");
export function dayLabel(date: string) {
  const now = new Date(),
    yesterday = new Date();
  yesterday.setDate(now.getDate() - 1);
  if (dayKey(date) === dayKey(now.toISOString())) return "Сегодня";
  if (dayKey(date) === dayKey(yesterday.toISOString())) return "Вчера";
  return new Date(date).toLocaleDateString("ru-RU", {
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}
