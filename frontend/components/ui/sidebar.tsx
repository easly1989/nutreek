"use client";

import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import {
  Home,
  Calendar,
  ShoppingCart,
  Users,
  Settings,
  Menu,
  X,
  ChefHat,
  Target,
  BarChart3,
  Bell,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "../../lib/utils";

interface SidebarProps {
  className?: string;
}

const navigationItems = [
  { name: "Dashboard", href: "/dashboard", icon: Home },
  { name: "Weekly Planner", href: "/dashboard/planner", icon: Calendar },
  { name: "Recipes", href: "/dashboard/recipes", icon: ChefHat },
  { name: "Ingredients", href: "/dashboard/ingredients", icon: Target },
  { name: "Shopping List", href: "/dashboard/shopping", icon: ShoppingCart },
  { name: "Household", href: "/dashboard/households", icon: Users },
  { name: "Analytics", href: "/dashboard/analytics", icon: BarChart3 },
  { name: "Settings", href: "/dashboard/settings", icon: Settings },
];

export function Sidebar({ className }: SidebarProps) {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const pathname = usePathname();

  return (
    <>
      {/* Mobile overlay */}
      <AnimatePresence>
        {!isCollapsed && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-40 bg-black/50 lg:hidden"
            onClick={() => setIsCollapsed(true)}
          />
        )}
      </AnimatePresence>

      {/* Sidebar */}
      <motion.div
        initial={{ x: -300 }}
        animate={{ x: isCollapsed ? -300 : 0 }}
        transition={{ type: "spring", stiffness: 300, damping: 30 }}
        className={cn(
          "fixed left-0 top-0 z-50 h-full w-64 bg-card border-r border-border shadow-lg lg:translate-x-0",
          isCollapsed && "lg:w-16",
          className
        )}
      >
        {/* Header */}
        <div className="flex h-16 items-center justify-between px-4 border-b border-border">
          <motion.div
            animate={{ opacity: isCollapsed ? 0 : 1 }}
            className="flex items-center space-x-2"
          >
            <motion.div
              whileHover={{ rotate: 360 }}
              transition={{ duration: 0.5 }}
              className="w-8 h-8 bg-primary rounded-lg flex items-center justify-center"
            >
              <ChefHat className="w-5 h-5 text-primary-foreground" />
            </motion.div>
            <span className="text-xl font-bold bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
              Nutreek
            </span>
          </motion.div>
          <button
            onClick={() => setIsCollapsed(!isCollapsed)}
            className="p-1.5 rounded-lg hover:bg-muted transition-colors duration-200"
          >
            <Menu className="w-5 h-5" />
          </button>
        </div>

        {/* Navigation */}
        <nav className="flex-1 px-3 py-4 space-y-1">
          {navigationItems.map((item) => {
            const Icon = item.icon;
            const isActive = pathname === item.href;

            return (
              <Link key={item.name} href={item.href}>
                <motion.div
                  whileHover={{ scale: 1.02 }}
                  whileTap={{ scale: 0.98 }}
                  className={cn(
                    "flex items-center px-3 py-2 rounded-lg text-sm font-medium transition-all duration-200 group",
                    isActive
                      ? "bg-primary text-primary-foreground shadow-md"
                      : "text-muted-foreground hover:bg-muted hover:text-foreground"
                  )}
                >
                  <Icon className="w-5 h-5 flex-shrink-0" />
                  <AnimatePresence>
                    {!isCollapsed && (
                      <motion.span
                        initial={{ opacity: 0, width: 0 }}
                        animate={{ opacity: 1, width: "auto" }}
                        exit={{ opacity: 0, width: 0 }}
                        transition={{ duration: 0.2 }}
                        className="ml-3 whitespace-nowrap"
                      >
                        {item.name}
                      </motion.span>
                    )}
                  </AnimatePresence>
                </motion.div>
              </Link>
            );
          })}
        </nav>

        {/* Footer */}
        <div className="p-4 border-t border-border">
          <motion.div
            animate={{ opacity: isCollapsed ? 0 : 1 }}
            className="flex items-center space-x-3"
          >
            <div className="w-8 h-8 bg-secondary rounded-lg flex items-center justify-center">
              <span className="text-sm font-semibold text-secondary-foreground">JD</span>
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-foreground truncate">John Doe</p>
              <p className="text-xs text-muted-foreground truncate">john@example.com</p>
            </div>
          </motion.div>
        </div>
      </motion.div>
    </>
  );
}

export function SidebarTrigger() {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <button
      onClick={() => setIsOpen(!isOpen)}
      className="p-2 rounded-lg hover:bg-muted transition-colors duration-200 lg:hidden"
    >
      <Menu className="w-5 h-5" />
    </button>
  );
}