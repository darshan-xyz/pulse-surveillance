import React from "react";
import { useState } from "react";
import { Check } from "lucide-react";

interface AnimatedCheckboxProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: React.ReactNode;
  error?: string;
  disabled?: boolean;
}

export function AnimatedCheckbox({
  checked,
  onChange,
  label,
  error,
  disabled,
}: AnimatedCheckboxProps) {
  const [hovered, setHovered] = useState(false);

  return (
    <div className="space-y-2">
      <label
        className={`flex items-start gap-3 cursor-pointer group ${
          disabled ? "opacity-50 cursor-not-allowed" : ""
        }`}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      >
        <div className="relative mt-0.5">
          <input
            type="checkbox"
            checked={checked}
            onChange={(e) => onChange(e.target.checked)}
            disabled={disabled}
            className="sr-only"
          />
          <div
            className={`w-5 h-5 rounded border-2 transition-all duration-200 flex items-center justify-center ${
              checked
                ? "bg-blue-600 dark:bg-neon-cyan border-blue-600 dark:border-neon-cyan scale-110"
                : "border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900"
            } ${
              hovered && !disabled
                ? "shadow-lg ring-2 ring-blue-500/20 dark:ring-neon-cyan/30"
                : ""
            }`}
          >
            <Check
              className={`w-3 h-3 text-white transition-all duration-200 ${
                checked ? "opacity-100 scale-100" : "opacity-0 scale-50"
              }`}
            />
          </div>

          {/* Ripple effect */}
          {hovered && !disabled && (
            <div className="absolute inset-0 bg-blue-500/20 dark:bg-neon-cyan/20 rounded-full animate-ping" />
          )}
        </div>

        <div className="text-sm leading-5">{label}</div>
      </label>

      {error && (
        <p className="text-sm text-red-600 dark:text-red-400 animate-in slide-in-from-left-2 duration-200 ml-8">
          {error}
        </p>
      )}
    </div>
  );
}
