import React, { useRef } from "react";
import { motion, useAnimation, useInView } from "framer-motion";
import { SurveillanceLogo } from "@/components/surveillance-logo";
import { ArrowLeft, Globe, Shield, Mail, Phone, MapPin, Facebook, Twitter, Linkedin, Instagram } from "lucide-react";
import { ParallaxBackground } from "@/components/ParallaxBackground";
import { ScrollToTopButton } from "@/components/ScrollToTopButton";
import { ThemeToggle } from "@/components/theme-toggle";

const team = [
  { name: "Alex Johnson", role: "CEO & Founder" },
  { name: "Jamie Lee", role: "CTO" },
  { name: "Morgan Smith", role: "Lead Engineer" },
  { name: "Taylor Kim", role: "Head of Customer Success" },
];

function AnimatedSection({ children, delay = 0, ...props }) {
  const ref = useRef(null);
  const inView = useInView(ref, { once: true, margin: "-100px" });
  const controls = useAnimation();
  React.useEffect(() => {
    if (inView) {
      controls.start({ opacity: 1, y: 0, scale: 1 });
    }
  }, [inView, controls]);
  return (
    <motion.div
      ref={ref}
      initial={{ opacity: 0, y: 60, scale: 0.98 }}
      animate={controls}
      transition={{ duration: 0.8, delay }}
      {...props}
    >
      {children}
    </motion.div>
  );
}

function AnimatedCard({ children, delay = 0, ...props }) {
  const ref = useRef(null);
  const inView = useInView(ref, { once: true, margin: "-80px" });
  const controls = useAnimation();
  React.useEffect(() => {
    if (inView) {
      controls.start({ opacity: 1, y: 0, scale: 1 });
    }
  }, [inView, controls]);
  return (
    <motion.div
      ref={ref}
      initial={{ opacity: 0, y: 40, scale: 0.97 }}
      animate={controls}
      transition={{ duration: 0.7, delay, type: "spring" }}
      {...props}
    >
      {children}
    </motion.div>
  );
}

export default function About() {
  return (
    <ParallaxBackground>
      <div className="relative min-h-screen w-full flex flex-col items-center justify-center overflow-hidden bg-white dark:bg-gray-900 transition-colors duration-300">
        {/* Theme Toggle - Fixed Position */}
        <div className="fixed top-6 right-6 z-50">
          <div className="p-2 bg-white/10 dark:bg-gray-900/10 backdrop-blur-lg rounded-full border border-white/20 dark:border-gray-700/20">
            <ThemeToggle />
          </div>
        </div>
        {/* Back Button */}
        <button
          onClick={() => window.history.back()}
          className="fixed top-6 left-6 z-50 flex items-center gap-2 px-4 py-2 bg-white/80 dark:bg-gray-900/80 border border-blue-200/40 dark:border-gray-700/40 rounded-full shadow-lg hover:bg-blue-100 dark:hover:bg-blue-800/60 text-blue-900 dark:text-blue-100 font-semibold transition-all duration-200 backdrop-blur-md"
          style={{ backdropFilter: 'blur(8px)' }}
          aria-label="Go back"
        >
          <ArrowLeft className="h-5 w-5" />
          <span className="hidden sm:inline">Back</span>
        </button>
        {/* Animated Gradient Background Blobs */}
        <motion.div
          className="absolute -top-32 -left-32 w-[40vw] h-[40vw] bg-cyan-400/30 rounded-full blur-3xl z-0"
          animate={{ x: [0, 60, 0], y: [0, 40, 0] }}
          transition={{ duration: 18, repeat: Infinity, ease: "easeInOut" }}
        />
        <motion.div
          className="absolute top-1/2 right-0 w-[30vw] h-[30vw] bg-purple-500/20 rounded-full blur-2xl z-0"
          animate={{ x: [0, -40, 0], y: [0, -60, 0] }}
          transition={{ duration: 22, repeat: Infinity, ease: "easeInOut" }}
        />
        <motion.div
          className="absolute bottom-0 left-1/3 w-[25vw] h-[25vw] bg-blue-600/20 rounded-full blur-2xl z-0"
          animate={{ x: [0, 30, 0], y: [0, -30, 0] }}
          transition={{ duration: 20, repeat: Infinity, ease: "easeInOut" }}
        />

        {/* Hero Section with scroll animation */}
        <div className="relative z-10 flex flex-col items-center justify-center min-h-[60vh] w-full text-center pt-24 pb-12 px-4">
          <div className="mb-6 flex items-center justify-center">
            <div className="mx-auto flex items-center justify-center bg-white/10 dark:bg-gray-900/30 rounded-2xl p-6 shadow-2xl backdrop-blur-xl border border-cyan-400/20">
              <SurveillanceLogo className="h-24 w-24" />
            </div>
          </div>
          <h1 className="text-5xl md:text-6xl font-extrabold bg-gradient-to-r from-cyan-700 via-blue-700 to-purple-700 dark:from-blue-200 dark:via-cyan-300 dark:to-purple-200 bg-clip-text text-transparent drop-shadow-lg mb-4">
            About METAVISION
          </h1>
          <p className="max-w-2xl mx-auto text-lg md:text-2xl text-blue-900 dark:text-blue-200/80 font-medium mb-6">
            We are a forward-thinking security company that helps businesses and homeowners protect their assets through innovative AI-powered surveillance solutions.
          </p>
        </div>

        {/* Glassmorphic Cards Section with scroll animations */}
        <section className="relative z-10 w-full flex flex-col md:flex-row items-center justify-center gap-8 md:gap-10 px-4 pb-24">
          <div className="flex-1 flex flex-col items-center justify-center backdrop-blur-xl bg-white/10 dark:bg-gray-900/30 border border-cyan-400/20 rounded-2xl shadow-2xl p-8 max-w-md w-full mb-8 md:mb-0">
            <h2 className="text-2xl font-bold text-cyan-700 dark:text-cyan-200 mb-2 text-center">Our Mission</h2>
            <p className="text-blue-900 dark:text-blue-200/80 text-center">
              To provide cutting-edge, reliable, and accessible security solutions that empower our clients to safeguard what matters most.
            </p>
          </div>
          <div className="flex-1 flex flex-col items-center justify-center backdrop-blur-xl bg-white/10 dark:bg-gray-900/30 border border-purple-400/20 rounded-2xl shadow-2xl p-8 max-w-md w-full mb-8 md:mb-0">
            <h2 className="text-2xl font-bold text-purple-700 dark:text-purple-200 mb-2 text-center">Our Vision</h2>
            <p className="text-blue-900 dark:text-blue-200/80 text-center">
              To be the global leader in AI-driven surveillance, setting new standards for safety, innovation, and customer trust.
            </p>
          </div>
        </section>
        <ScrollToTopButton />
        {/* Advanced Footer */}
        <footer className="relative z-20 bg-white/80 dark:bg-gray-900/80 backdrop-blur-md border-t border-blue-200/50 dark:border-gray-700/50 mt-16">
          <div className="container mx-auto px-4 py-12 grid grid-cols-1 md:grid-cols-4 gap-10">
            {/* Company Info */}
            <div className="flex flex-col items-start gap-4">
              <div className="flex items-center gap-3">
                <SurveillanceLogo className="h-10 w-10" />
                <span className="text-2xl font-black text-blue-900 dark:text-blue-200 tracking-wide">METAVISION</span>
              </div>
              <p className="text-gray-600 dark:text-gray-400 max-w-xs text-sm">
                Advanced AI-powered surveillance for a safer, smarter world. Protecting your assets with innovation and trust.
              </p>
              <div className="flex gap-3 mt-2">
                <a href="https://facebook.com" target="_blank" rel="noopener" className="p-2 rounded-full bg-blue-100 dark:bg-blue-900/30 hover:bg-blue-200 dark:hover:bg-blue-800 transition-colors"><Facebook className="h-5 w-5 text-blue-600 dark:text-blue-300" /></a>
                <a href="https://twitter.com" target="_blank" rel="noopener" className="p-2 rounded-full bg-blue-100 dark:bg-blue-900/30 hover:bg-blue-200 dark:hover:bg-blue-800 transition-colors"><Twitter className="h-5 w-5 text-blue-500 dark:text-blue-200" /></a>
                <a href="https://linkedin.com" target="_blank" rel="noopener" className="p-2 rounded-full bg-blue-100 dark:bg-blue-900/30 hover:bg-blue-200 dark:hover:bg-blue-800 transition-colors"><Linkedin className="h-5 w-5 text-blue-700 dark:text-blue-100" /></a>
                <a href="https://instagram.com" target="_blank" rel="noopener" className="p-2 rounded-full bg-pink-100 dark:bg-pink-900/30 hover:bg-pink-200 dark:hover:bg-pink-800 transition-colors"><Instagram className="h-5 w-5 text-pink-600 dark:text-pink-200" /></a>
              </div>
            </div>
            {/* Navigation Links */}
            <div className="flex flex-col gap-2">
              <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">Quick Links</h4>
              <a href="/" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 transition-colors">Home</a>
              <a href="/about" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 transition-colors">About Us</a>
              <a href="/history" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 transition-colors">Detection History</a>
              <a href="/login" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 transition-colors">Login</a>
            </div>
            {/* Contact Info */}
            <div className="flex flex-col gap-2">
              <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">Contact</h4>
              <div className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"><Mail className="h-4 w-4" /> support@metavision.com</div>
              <div className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"><Phone className="h-4 w-4" /> +1 234 567 8901</div>
              <div className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"><MapPin className="h-4 w-4" /> 123 Tech Street, Innovation City, USA</div>
            </div>
            {/* Newsletter Signup */}
            <div className="flex flex-col gap-3">
              <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">Newsletter</h4>
              <p className="text-sm text-gray-600 dark:text-gray-400">Stay updated with the latest in AI security.</p>
              <form className="flex gap-2 mt-1">
                <input type="email" required placeholder="Your email" className="rounded-lg px-3 py-2 bg-white/80 dark:bg-gray-800/80 border border-blue-200 dark:border-gray-700 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400 transition" />
                <button type="submit" className="px-4 py-2 rounded-lg bg-blue-600 text-white font-semibold hover:bg-blue-700 transition">Subscribe</button>
              </form>
            </div>
          </div>
          <div className="border-t border-blue-200/50 dark:border-gray-700/50 mt-8 pt-6 pb-2 text-center text-xs text-gray-500 dark:text-gray-400 bg-white/60 dark:bg-gray-900/60">
            <span>© 2025 METAVISION. All rights reserved. | Enterprise Grade Security <Shield className="inline h-3 w-3 ml-1" /></span>
          </div>
        </footer>
      </div>
    </ParallaxBackground>
  );
} 