import React, { useEffect, useState } from "react";
import { ArrowUp } from "lucide-react";

export const ScrollToTopButton: React.FC = () => {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const onScroll = () => {
      setVisible(window.scrollY > 200);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const scrollToTop = () => {
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  return (
    <button
      onClick={scrollToTop}
      className={`fixed bottom-8 right-8 z-50 p-3 rounded-full shadow-lg bg-blue-600 text-white dark:bg-neon-cyan/80 dark:text-gray-900 hover:bg-blue-700 dark:hover:bg-neon-cyan transition-all duration-300 ${visible ? 'opacity-100 scale-100' : 'opacity-0 scale-90 pointer-events-none'}`}
      aria-label="Scroll to top"
      style={{ transition: 'opacity 0.3s, transform 0.3s' }}
    >
      <ArrowUp className="h-5 w-5" />
    </button>
  );
}; 