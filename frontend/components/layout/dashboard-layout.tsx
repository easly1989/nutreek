"use client";

import { useState } from "react";
import { motion } from "framer-motion";
import { Sidebar } from "../ui/sidebar";
import { Bell, Search } from "lucide-react";
import { useSkipLink, useAriaExpanded } from "../../hooks/use-accessibility";

interface DashboardLayoutProps {
  children: React.ReactNode;
}

export function DashboardLayout({ children }: DashboardLayoutProps) {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const skipToMain = useSkipLink('main-content');
  const skipToNav = useSkipLink('main-navigation');

  return (
    <div className="min-h-screen bg-background">
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
    </div>
  );
}