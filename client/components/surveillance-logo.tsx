import React from "react";

export function SurveillanceLogo({
  className = "h-8 w-8",
}: {
  className?: string;
}) {
  return (
    <div className={`relative ${className} overflow-hidden`}>
      <img
        src="/image.png"
        alt="METAVISION Logo"
        className="w-full h-full object-cover rounded-2xl scale-125"
      />
    </div>
  );
}
