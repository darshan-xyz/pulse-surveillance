import React from "react";
import { Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useEffect, useState } from "react";

export function ThemeToggle() {
  const [theme, setTheme] = useState<"light" | "dark">("light");

  useEffect(() => {
    const savedTheme = localStorage.getItem("theme") as "light" | "dark" | null;
    const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
      .matches
      ? "dark"
      : "light";

    const initialTheme = savedTheme || systemTheme;
    setTheme(initialTheme);

    if (initialTheme === "dark") {
      document.documentElement.classList.add("dark");
    }
  }, []);

  const toggleTheme = () => {
    const newTheme = theme === "light" ? "dark" : "light";
    setTheme(newTheme);
    localStorage.setItem("theme", newTheme);

    if (newTheme === "dark") {
      document.documentElement.classList.add("dark");
    } else {
      document.documentElement.classList.remove("dark");
    }
  };

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={toggleTheme}
      className="h-9 w-9 p-0 hover:bg-blue-100 dark:hover:bg-neon-cyan/10 transition-all duration-500 ease-out group"
    >
      <div className="relative">
        {theme === "light" ? (
          <Moon className="h-4 w-4 transition-all duration-500 group-hover:rotate-12 group-hover:scale-110" />
        ) : (
          <Sun className="h-4 w-4 transition-all duration-500 group-hover:rotate-180 group-hover:scale-110" />
        )}
      </div>
      <span className="sr-only">Toggle theme</span>
    </Button>
  );
}
