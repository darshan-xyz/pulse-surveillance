import React from "react";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { SurveillanceLogo } from "@/components/surveillance-logo";
import { Mail, Phone, MapPin, Facebook, Twitter, Linkedin, Instagram, Shield } from "lucide-react";

export default function Terms() {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-gradient-to-br from-green-100 via-emerald-200 to-teal-100 dark:from-slate-900 dark:via-green-900 dark:to-slate-900 p-4">
      {/* Header */}
      <header className="w-full bg-white/80 dark:bg-gray-900/80 backdrop-blur-md border-b border-blue-200/50 dark:border-gray-700/50 py-4 px-4 flex flex-col md:flex-row items-center justify-between gap-4 z-20">
        <div className="flex items-center gap-3">
          <SurveillanceLogo className="h-8 w-8" />
          <span className="font-black text-xl text-blue-900 dark:text-blue-200 tracking-wide">CYBER EAGLE</span>
        </div>
        <nav className="flex gap-6 items-center">
          <a href="/about" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 font-medium transition-colors">About</a>
          <a href="/privacy" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 font-medium transition-colors">Privacy</a>
          <a href="/login" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 font-medium transition-colors">Login</a>
        </nav>
      </header>
      <div className="mt-8" />
      <div className="flex-1 w-full flex items-center justify-center">
        <Card className="w-full max-w-2xl bg-white/90 dark:bg-gray-900/90 shadow-xl border-0">
          <CardHeader className="text-center pb-2">
            <h1 className="text-3xl font-black bg-gradient-to-r from-green-700 to-emerald-700 dark:from-green-300 dark:to-emerald-300 bg-clip-text text-transparent tracking-wide mb-2">
              Terms and Conditions
            </h1>
          </CardHeader>
          <CardContent className="space-y-6 text-gray-800 dark:text-gray-200 text-base">
            <section>
              <h2 className="text-xl font-bold mb-2">1. Acceptance of Terms</h2>
              <p>By accessing or using Cyber Eagle Surveillance, you agree to be bound by these Terms and Conditions. If you do not agree, please do not use our service.</p>
            </section>
            <section>
              <h2 className="text-xl font-bold mb-2">2. Use of Service</h2>
              <p>You may use the service only for lawful purposes and in accordance with these Terms. You are responsible for your account and all activity conducted under it.</p>
            </section>
            <section>
              <h2 className="text-xl font-bold mb-2">3. User Responsibilities</h2>
              <p>You agree not to misuse the service, attempt unauthorized access, or interfere with the operation of the platform. You are responsible for maintaining the confidentiality of your login credentials.</p>
            </section>
            <section>
              <h2 className="text-xl font-bold mb-2">4. Limitation of Liability</h2>
              <p>Cyber Eagle Surveillance is provided "as is" without warranties of any kind. We are not liable for any damages or losses resulting from your use of the service.</p>
            </section>
            <section>
              <h2 className="text-xl font-bold mb-2">5. Changes to Terms</h2>
              <p>We may update these Terms at any time. Continued use of the service after changes constitutes acceptance of the new Terms.</p>
            </section>
            <section>
              <h2 className="text-xl font-bold mb-2">6. Contact</h2>
              <p>If you have any questions about these Terms, please contact us at <a href="mailto:support@cybereagle.com" className="text-blue-600 underline">support@cybereagle.com</a>.</p>
            </section>
          </CardContent>
        </Card>
      </div>
      {/* Advanced Footer (copied from About Us page) */}
      <footer className="relative z-20 bg-white/80 dark:bg-gray-900/80 backdrop-blur-md border-t border-blue-200/50 dark:border-gray-700/50 mt-16">
        <div className="container mx-auto px-4 py-12 grid grid-cols-1 md:grid-cols-4 gap-10">
          {/* Company Info */}
          <div className="flex flex-col items-start gap-4">
            <div className="flex items-center gap-3">
              <SurveillanceLogo className="h-10 w-10" />
              <span className="text-2xl font-black text-blue-900 dark:text-blue-200 tracking-wide">CYBER EAGLE</span>
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
            <a href="/about" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 transition-colors">About Us</a>
            <a href="/privacy" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 transition-colors">Privacy</a>
            <a href="/login" className="text-sm text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-300 transition-colors">Login</a>
          </div>
          {/* Contact Info */}
          <div className="flex flex-col gap-2">
            <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">Contact</h4>
            <div className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"><Mail className="h-4 w-4" /> support@cybereagle.com</div>
            <div className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"><Phone className="h-4 w-4" /> +1 234 567 8901</div>
            <div className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"><MapPin className="h-4 w-4" /> 123 Eagle St, Tech City, USA</div>
          </div>
          {/* Newsletter Signup */}
          <div className="flex flex-col gap-3">
            <h4 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">Newsletter</h4>
            <p className="text-sm text-gray-600 dark:text-gray-400">Stay updated with the latest in AI security.</p>
            <form className="flex flex-col sm:flex-row gap-2 mt-1 w-full">
              <input type="email" required placeholder="Your email" className="flex-1 rounded-lg px-3 py-2 bg-white/80 dark:bg-gray-800/80 border border-blue-200 dark:border-gray-700 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400 transition min-w-0" />
              <button type="submit" className="px-4 py-2 rounded-lg bg-blue-600 text-white font-semibold hover:bg-blue-700 transition w-full sm:w-auto">Subscribe</button>
            </form>
          </div>
        </div>
        <div className="border-t border-blue-200/50 dark:border-gray-700/50 mt-8 pt-6 pb-2 text-center text-xs text-gray-500 dark:text-gray-400 bg-white/60 dark:bg-gray-900/60">
          <span>© {new Date().getFullYear()} CYBER EAGLE. All rights reserved. | Enterprise Grade Security <Shield className="inline h-3 w-3 ml-1" /></span>
        </div>
      </footer>
    </div>
  );
} 