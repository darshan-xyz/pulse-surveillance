import React from "react";
import { useState } from "react";
import { Eye, EyeOff, Lock } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { getPasswordStrength } from "@/lib/auth-utils";

interface PasswordInputProps {
  label: string;
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
  error?: string;
  disabled?: boolean;
  showStrength?: boolean;
  id?: string;
}

export function PasswordInput({
  label,
  placeholder,
  value,
  onChange,
  error,
  disabled,
  showStrength = false,
  id = "password",
}: PasswordInputProps) {
  const [showPassword, setShowPassword] = useState(false);
  const [focused, setFocused] = useState(false);

  const passwordStrength = showStrength ? getPasswordStrength(value) : null;

  return (
    <div className="space-y-2">
      <Label htmlFor={id} className="text-sm font-medium">
        {label}
      </Label>
      <div className="relative">
        <Input
          id={id}
          type={showPassword ? "text" : "password"}
          placeholder={placeholder}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          disabled={disabled}
          className={`pr-12 pl-10 transition-all duration-200 ${
            focused
              ? "scale-[1.02] shadow-md ring-2 ring-blue-500/20 dark:ring-neon-cyan/30"
              : ""
          } ${
            error
              ? "border-red-500 dark:border-red-400"
              : "border-gray-300 dark:border-gray-600"
          }`}
        />
        <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setShowPassword(!showPassword)}
          className="absolute right-1 top-1/2 h-8 w-8 -translate-y-1/2 p-0 hover:bg-gray-100 dark:hover:bg-gray-800"
          disabled={disabled}
        >
          {showPassword ? (
            <EyeOff className="h-4 w-4" />
          ) : (
            <Eye className="h-4 w-4" />
          )}
        </Button>
      </div>

      {/* Password Strength Indicator */}
      {showStrength && value && (
        <div className="space-y-1 animate-in slide-in-from-bottom-2 duration-300">
          <div className="flex items-center gap-2">
            <div className="flex-1 h-1 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
              <div
                className={`h-full transition-all duration-300 ${
                  passwordStrength.score >= 1 ? "bg-red-500" : ""
                } ${passwordStrength.score >= 2 ? "bg-orange-500" : ""} ${
                  passwordStrength.score >= 3 ? "bg-yellow-500" : ""
                } ${passwordStrength.score >= 4 ? "bg-blue-500" : ""} ${
                  passwordStrength.score >= 5 ? "bg-green-500" : ""
                }`}
                style={{ width: `${(passwordStrength.score / 5) * 100}%` }}
              />
            </div>
            <span className={`text-xs font-medium ${passwordStrength.color}`}>
              {passwordStrength.label}
            </span>
          </div>
        </div>
      )}

      {error && (
        <p className="text-sm text-red-600 dark:text-red-400 animate-in slide-in-from-left-2 duration-200">
          {error}
        </p>
      )}
    </div>
  );
}
