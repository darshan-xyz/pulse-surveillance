import React from "react";

export const ParallaxBackground: React.FC<{ children?: React.ReactNode }> = ({ children }) => {
  return (
    <div className="relative w-full min-h-screen overflow-x-hidden bg-white dark:bg-gray-900 transition-colors duration-300">
      {/* Static Gradient Blobs (no parallax) */}
      <div className="pointer-events-none absolute -top-32 -left-32 w-[40vw] h-[40vw] bg-cyan-400/30 rounded-full blur-3xl z-0 transition-transform duration-300" />
      <div className="pointer-events-none absolute top-1/2 right-0 w-[30vw] h-[30vw] bg-purple-500/20 rounded-full blur-2xl z-0 transition-transform duration-300" />
      <div className="pointer-events-none absolute bottom-0 left-1/3 w-[25vw] h-[25vw] bg-blue-600/20 rounded-full blur-2xl z-0 transition-transform duration-300" />
      <div className="relative z-10">{children}</div>
    </div>
  );
}; 