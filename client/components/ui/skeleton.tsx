import React from "react";
import { cn } from "@/lib/utils";

function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "relative overflow-hidden bg-muted dark:bg-muted rounded-md",
        "before:absolute before:inset-0 before:-translate-x-full before:animate-shimmer before:bg-gradient-to-r before:from-transparent before:via-white/60 dark:before:via-white/20 before:to-transparent",
        className
      )}
      {...props}
    />
  );
}

// Add shimmer animation to global CSS (if not present)
// @layer utilities {
//   @keyframes shimmer {
//     100% { transform: translateX(100%); }
//   }
//   .animate-shimmer {
//     animation: shimmer 1.5s infinite linear;
//   }
// }

export { Skeleton };
