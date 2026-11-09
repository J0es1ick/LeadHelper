import { useEffect, useState } from "react";
import { Sun, Moon, Monitor } from "lucide-react";
type Theme = "light" | "dark" | "system";
const key = "leadhelper.theme";
let fallback: Theme = "system";
const preference = (): Theme => {
  try {
    const t = localStorage.getItem(key);
    return t === "light" || t === "dark" ? t : "system";
  } catch {
    return fallback;
  }
};
export function applyTheme() {
  const theme = preference();
  document.documentElement.dataset.theme =
    theme === "system"
      ? matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light"
      : theme;
}
applyTheme();
function useTheme() {
  const [theme, setTheme] = useState<Theme>(preference);
  const [systemDark, setSystemDark] = useState(
    () => matchMedia("(prefers-color-scheme: dark)").matches,
  );
  useEffect(() => {
    const media = matchMedia("(prefers-color-scheme: dark)");
    const sync = () => {
      setTheme(preference());
      setSystemDark(media.matches);
      applyTheme();
    };
    window.addEventListener("leadhelper-theme", sync);
    window.addEventListener("storage", sync);
    media.addEventListener("change", sync);
    return () => {
      window.removeEventListener("leadhelper-theme", sync);
      window.removeEventListener("storage", sync);
      media.removeEventListener("change", sync);
    };
  }, []);
  const change = (value: Theme) => {
    fallback = value;
    try {
      localStorage.setItem(key, value);
    } catch {
      /* Theme still applies in this tab. */
    }
    setTheme(value);
    document.documentElement.dataset.theme =
      value === "system"
        ? matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light"
        : value;
    window.dispatchEvent(new Event("leadhelper-theme"));
  };
  return { theme, change, systemDark };
}
export function ThemeChooser() {
  const { theme, change } = useTheme();
  return (
    <div className="theme-options" role="group" aria-label="Тема оформления">
      {(
        [
          { id: "light", name: "Светлая", icon: Sun },
          { id: "dark", name: "Тёмная", icon: Moon },
          { id: "system", name: "Как в системе", icon: Monitor },
        ] as const
      ).map((t) => (
        <button
          type="button"
          className={"theme-option " + (theme === t.id ? "active" : "")}
          aria-pressed={theme === t.id}
          key={t.id}
          onClick={() => change(t.id)}
        >
          <t.icon size={22} />
          <span>{t.name}</span>
        </button>
      ))}
    </div>
  );
}
export function ThemeToggle() {
  const { theme, change, systemDark } = useTheme();
  const dark = theme === "dark" || (theme === "system" && systemDark);
  return (
    <button
      className="icon-button"
      aria-label={dark ? "Включить светлую тему" : "Включить тёмную тему"}
      title={dark ? "Светлая тема" : "Тёмная тема"}
      onClick={() => change(dark ? "light" : "dark")}
    >
      {dark ? <Sun size={20} /> : <Moon size={20} />}
    </button>
  );
}
