import React from "react";
import { useState } from "react";
import { ChevronDown, Phone } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { countryCodes } from "@/lib/auth-utils";

interface PhoneInputProps {
  value: { countryCode: string; number: string };
  onChange: (value: { countryCode: string; number: string }) => void;
  error?: string;
  disabled?: boolean;
}

export function PhoneInput({
  value,
  onChange,
  error,
  disabled,
}: PhoneInputProps) {
  const [focused, setFocused] = useState(false);

  const selectedCountry =
    countryCodes.find((c) =>
      value.countryCode.includes("-")
        ? `${c.code}-${c.country}` === value.countryCode
        : c.code === value.countryCode,
    ) || countryCodes[0];

  return (
    <div className="space-y-2">
      <Label htmlFor="phone" className="text-sm font-medium">
        Phone Number
      </Label>
      <div className="relative">
        <div className="flex">
          <Select
            value={
              value.countryCode.includes("-")
                ? value.countryCode
                : `${value.countryCode}-US`
            }
            onValueChange={(selectedValue) => {
              onChange({ ...value, countryCode: selectedValue });
            }}
          >
            <SelectTrigger className="w-[120px] rounded-r-none border-r-0 bg-white dark:bg-gray-900">
              <div className="flex items-center gap-2">
                <span className="text-lg">{selectedCountry.flag}</span>
                <span className="text-sm font-mono">
                  {selectedCountry.code}
                </span>
              </div>
            </SelectTrigger>
            <SelectContent>
              {countryCodes.map((country, index) => (
                <SelectItem
                  key={`${country.code}-${country.country}-${index}`}
                  value={`${country.code}-${country.country}`}
                >
                  <div className="flex items-center gap-2">
                    <span className="text-lg">{country.flag}</span>
                    <span className="font-mono text-sm">{country.code}</span>
                    <span className="text-sm text-gray-600 dark:text-gray-400">
                      {country.name}
                    </span>
                  </div>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <div className="relative flex-1">
            <Input
              id="phone"
              type="tel"
              placeholder="Enter phone number"
              value={value.number}
              onChange={(e) =>
                onChange({
                  ...value,
                  number: e.target.value.replace(/\D/g, ""),
                })
              }
              onFocus={() => setFocused(true)}
              onBlur={() => setFocused(false)}
              disabled={disabled}
              className={`rounded-l-none pl-4 transition-all duration-200 ${
                focused
                  ? "scale-[1.02] shadow-md ring-2 ring-blue-500/20 dark:ring-neon-cyan/30"
                  : ""
              } ${
                error
                  ? "border-red-500 dark:border-red-400"
                  : "border-gray-300 dark:border-gray-600"
              }`}
            />
            <Phone className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
          </div>
        </div>

        {error && (
          <p className="text-sm text-red-600 dark:text-red-400 animate-in slide-in-from-left-2 duration-200">
            {error}
          </p>
        )}
      </div>
    </div>
  );
}
