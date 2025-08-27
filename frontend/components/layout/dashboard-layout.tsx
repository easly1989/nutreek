"use client";

import { useState, useEffect } from "react";
import { motion } from "framer-motion";
import { Sidebar } from "../ui/sidebar";
import { Bell, Search, ChefHat } from "lucide-react";
import { useSkipLink, useAriaExpanded } from "../../hooks/use-accessibility";

interface DashboardLayoutProps {
  children: React.ReactNode;
}

export function DashboardLayout({ children }: DashboardLayoutProps) {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mousePosition, setMousePosition] = useState({ x: 0, y: 0 });
  const [scrollPosition, setScrollPosition] = useState(0);
  const [hoveredIcon, setHoveredIcon] = useState<number | null>(null);
  const skipToMain = useSkipLink('main-content');
  const skipToNav = useSkipLink('main-navigation');

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      setMousePosition({
        x: (e.clientX / window.innerWidth - 0.5) * 20,
        y: (e.clientY / window.innerHeight - 0.5) * 20,
      });
    };

    const handleScroll = () => {
      setScrollPosition(window.scrollY);
    };

    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('scroll', handleScroll);
    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('scroll', handleScroll);
    };
  }, []);

  return (
    <>
      {/* Fixed Gradient Background */}
      <div className="fixed inset-0 bg-gradient-to-b from-emerald-100 via-sky-100 to-cyan-100 pointer-events-none" />

      {/* Very Subtle Overlay for Text Contrast */}
      <div className="fixed inset-0 bg-slate-900/5 pointer-events-none" />

      {/* Floating Chef Icons Pattern Overlay */}
      <div className="fixed inset-0 overflow-hidden">
        {[
          { id: 1, top: '100px', left: '10%', size: 3, color: 'text-red-500', speed: 0.3 },
          { id: 2, top: '200px', right: '15%', size: 4, color: 'text-blue-500', speed: -0.4 },
          { id: 3, top: '300px', left: '25%', size: 5, color: 'text-green-500', speed: 0.5 },
          { id: 4, top: '400px', right: '30%', size: 3, color: 'text-purple-500', speed: -0.6 },
          { id: 5, top: '500px', left: '65%', size: 4, color: 'text-pink-500', speed: 0.7 },
          { id: 6, top: '150px', right: '75%', size: 6, color: 'text-teal-500', speed: -0.3 },
          { id: 7, top: '350px', left: '8%', size: 3, color: 'text-yellow-500', speed: 0.4 },
          { id: 8, top: '450px', right: '20%', size: 5, color: 'text-indigo-500', speed: -0.5 },
          { id: 9, top: '550px', left: '50%', size: 4, color: 'text-orange-500', speed: 0.6 },
          { id: 10, top: '250px', left: '15%', size: 6, color: 'text-emerald-500', speed: 0.8 },
          { id: 11, top: '350px', right: '10%', size: 4, color: 'text-violet-500', speed: -0.2 },
          { id: 12, top: '450px', left: '75%', size: 3, color: 'text-rose-500', speed: 0.3 },
          { id: 13, top: '550px', right: '25%', size: 5, color: 'text-lime-500', speed: -0.4 },
          { id: 14, top: '200px', left: '12%', size: 4, color: 'text-sky-500', speed: 0.5 },
          { id: 15, top: '300px', right: '60%', size: 3, color: 'text-fuchsia-500', speed: -0.6 },
          { id: 16, top: '400px', left: '40%', size: 6, color: 'text-amber-500', speed: 0.7 },
          { id: 17, top: '500px', right: '18%', size: 4, color: 'text-slate-500', speed: -0.3 },
          { id: 18, top: '600px', left: '22%', size: 6, color: 'text-orange-500', speed: 0.8 },
          { id: 19, top: '650px', right: '22%', size: 4, color: 'text-violet-500', speed: -0.2 },
          { id: 20, top: '700px', left: '40%', size: 3, color: 'text-rose-500', speed: 0.4 }
        ].map((icon) => {
          const baseSize = icon.size * 4;
          const hoverSize = baseSize + 8;

          return (
            <div
              key={icon.id}
              className="absolute transition-all duration-300 cursor-pointer group"
              style={{
                top: icon.top,
                left: icon.left,
                right: icon.right,
                transform: `translate(${mousePosition.x * icon.speed}px, ${mousePosition.y * icon.speed + scrollPosition * 0.1}px)`,
              }}
            >
              <ChefHat
                className={`${icon.color} opacity-20 group-hover:opacity-70 transition-all duration-300`}
                style={{
                  width: hoveredIcon === icon.id ? hoverSize : baseSize,
                  height: hoveredIcon === icon.id ? hoverSize : baseSize,
                }}
                onMouseEnter={() => setHoveredIcon(icon.id)}
                onMouseLeave={() => setHoveredIcon(null)}
              />
            </div>
          );
        })}
      </div>

      <div className="min-h-screen relative z-10">
        {/* Skip Links */}
        <a
          href="#main-content"
          onClick={skipToMain}
          className="skip-link"
        >
          Skip to main content
        </a>
        <a
          href="#main-navigation"
          onClick={skipToNav}
          className="skip-link"
        >
          Skip to main navigation
        </a>

        {/* Sidebar */}
        <Sidebar />

      {/* Main content */}
      <div className="lg:ml-64">
        {/* Top navigation */}
        <motion.header
          initial={{ y: -20, opacity: 0 }}
          animate={{ y: 0, opacity: 1 }}
          transition={{ duration: 0.3 }}
          className="sticky top-0 z-30 bg-card/80 backdrop-blur-md border-b border-border"
          role="banner"
        >
          <div className="flex h-16 items-center justify-between px-4 lg:px-6">
            {/* Mobile menu button */}
            <button
              onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
              className="focus-visible p-2 rounded-lg hover:bg-muted transition-colors duration-200 lg:hidden"
              aria-label="Toggle navigation menu"
              aria-expanded={!sidebarCollapsed}
              aria-controls="main-navigation"
            >
              <motion.div
                animate={{ rotate: sidebarCollapsed ? 0 : 180 }}
                transition={{ duration: 0.2 }}
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                </svg>
              </motion.div>
            </button>

            {/* Search bar */}
            <div className="flex-1 max-w-md mx-4">
              <motion.div
                initial={{ scale: 0.95, opacity: 0 }}
                animate={{ scale: 1, opacity: 1 }}
                transition={{ delay: 0.1, duration: 0.3 }}
                className="relative"
              >
                <label htmlFor="global-search" className="sr-only">
                  Search recipes, ingredients, and more
                </label>
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-muted-foreground" aria-hidden="true" />
                <input
                  id="global-search"
                  type="text"
                  placeholder="Search recipes, ingredients..."
                  className="focus-visible w-full pl-10 pr-4 py-2 bg-muted rounded-lg border border-border transition-colors duration-200"
                  aria-describedby="search-help"
                />
                <div id="search-help" className="sr-only">
                  Search across recipes, ingredients, meal plans, and more
                </div>
              </motion.div>
            </div>

            {/* Right side actions */}
            <nav className="flex items-center space-x-2" aria-label="User actions">
              {/* Notifications */}
              <motion.button
                whileHover={{ scale: 1.05 }}
                whileTap={{ scale: 0.95 }}
                className="focus-visible relative p-2 rounded-lg hover:bg-muted transition-colors duration-200"
                aria-label="Notifications"
                aria-describedby="notification-count"
              >
                <Bell className="w-5 h-5" aria-hidden="true" />
                <motion.span
                  initial={{ scale: 0 }}
                  animate={{ scale: 1 }}
                  className="absolute -top-1 -right-1 w-3 h-3 bg-destructive rounded-full flex items-center justify-center"
                  aria-hidden="true"
                >
                  <span className="text-xs text-destructive-foreground font-semibold">3</span>
                </motion.span>
                <div id="notification-count" className="sr-only">
                  You have 3 unread notifications
                </div>
              </motion.button>

              {/* User menu */}
              <motion.button
                whileHover={{ scale: 1.05 }}
                className="focus-visible w-8 h-8 bg-secondary rounded-lg flex items-center justify-center transition-colors duration-200"
                aria-label="User menu"
                aria-haspopup="menu"
                aria-expanded="false"
              >
                <span className="text-sm font-semibold text-secondary-foreground">JD</span>
              </motion.button>
            </nav>
          </div>
        </motion.header>

        {/* Page content */}
        <motion.main
          id="main-content"
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4 }}
          className="p-4 lg:p-6"
          role="main"
          aria-labelledby="page-title"
        >
          {children}
        </motion.main>
      </div>
    </>
  );
}