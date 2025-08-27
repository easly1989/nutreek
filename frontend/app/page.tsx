'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Users, Calendar, ShoppingCart, ArrowRight, Sparkles, ChefHat } from 'lucide-react'

export default function Home() {
  const [mousePosition, setMousePosition] = useState({ x: 0, y: 0 })
  const [scrollPosition, setScrollPosition] = useState(0)
  const [hoveredIcon, setHoveredIcon] = useState<number | null>(null)

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      setMousePosition({
        x: (e.clientX / window.innerWidth - 0.5) * 20,
        y: (e.clientY / window.innerHeight - 0.5) * 20,
      })
    }

    const handleScroll = () => {
      setScrollPosition(window.scrollY)
    }

    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('scroll', handleScroll)
    return () => {
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('scroll', handleScroll)
    }
  }, [])

  return (
    <main className="min-h-screen overflow-hidden relative">
      {/* Fixed Gradient Background */}
      <div className="fixed inset-0 bg-gradient-to-b from-emerald-100 via-sky-100 to-cyan-100 pointer-events-none" />

      {/* Very Subtle Overlay for Text Contrast */}
      <div className="fixed inset-0 bg-slate-900/5 pointer-events-none" />

      {/* Interactive Chef Icons Pattern Overlay */}
      <div className="fixed inset-0 overflow-hidden">
        {[
          { id: 1, top: '50px', left: '10%', size: 3, color: 'text-red-500', speed: 0.3 },
          { id: 2, top: '100px', right: '15%', size: 4, color: 'text-blue-500', speed: -0.4 },
          { id: 3, top: '150px', left: '25%', size: 5, color: 'text-green-500', speed: 0.5 },
          { id: 4, top: '200px', right: '30%', size: 3, color: 'text-purple-500', speed: -0.6 },
          { id: 5, top: '250px', left: '65%', size: 4, color: 'text-pink-500', speed: 0.7 },
          { id: 6, top: '300px', right: '75%', size: 6, color: 'text-teal-500', speed: -0.3 },
          { id: 7, top: '350px', left: '8%', size: 3, color: 'text-yellow-500', speed: 0.4 },
          { id: 8, top: '400px', right: '20%', size: 5, color: 'text-indigo-500', speed: -0.5 },
          { id: 9, top: '450px', left: '50%', size: 4, color: 'text-orange-500', speed: 0.6 },
          { id: 10, top: '200px', left: '15%', size: 6, color: 'text-emerald-500', speed: 0.8 },
          { id: 11, top: '250px', right: '10%', size: 4, color: 'text-violet-500', speed: -0.2 },
          { id: 12, top: '300px', left: '75%', size: 3, color: 'text-rose-500', speed: 0.3 },
          { id: 13, top: '350px', right: '25%', size: 5, color: 'text-lime-500', speed: -0.4 },
          { id: 14, top: '400px', left: '12%', size: 4, color: 'text-sky-500', speed: 0.5 },
          { id: 15, top: '450px', right: '60%', size: 3, color: 'text-fuchsia-500', speed: -0.6 },
          { id: 16, top: '500px', left: '40%', size: 6, color: 'text-amber-500', speed: 0.7 },
          { id: 17, top: '550px', right: '18%', size: 4, color: 'text-slate-500', speed: -0.3 },
          { id: 18, top: '600px', left: '22%', size: 6, color: 'text-orange-500', speed: 0.8 },
          { id: 19, top: '650px', right: '22%', size: 4, color: 'text-violet-500', speed: -0.2 },
          { id: 20, top: '700px', left: '40%', size: 3, color: 'text-rose-500', speed: 0.4 },
          { id: 21, top: '750px', right: '40%', size: 5, color: 'text-lime-500', speed: -0.5 },
          { id: 22, top: '550px', left: '8%', size: 5, color: 'text-green-500', speed: 0.4 },
          { id: 23, top: '600px', right: '12%', size: 3, color: 'text-blue-500', speed: -0.5 },
          { id: 24, top: '650px', left: '25%', size: 4, color: 'text-teal-500', speed: 0.6 },
          { id: 25, top: '700px', right: '50%', size: 6, color: 'text-purple-500', speed: -0.7 }
        ].map((icon) => {
          const baseSize = icon.size * 4; // Convert to pixels (3->12px, 4->16px, etc.)
          const hoverSize = baseSize + 8; // Add 8px for hover

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

      {/* Hero Section */}
      <section className="relative overflow-hidden">
        <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pt-20 pb-16">
          <div className="text-center">
            <div
              className="inline-flex items-center justify-center w-20 h-20 bg-gradient-to-br from-primary/20 to-accent/20 rounded-2xl mb-8 backdrop-blur-sm border border-white/10"
              style={{
                transform: `translate(${mousePosition.x * 0.1}px, ${mousePosition.y * 0.1}px)`,
              }}
            >
              <ChefHat className="w-10 h-10 text-primary drop-shadow-lg" />
            </div>

            <h1
              className="text-display bg-gradient-to-r from-slate-800 to-slate-600 bg-clip-text text-transparent mb-4 drop-shadow-sm"
              style={{
                transform: `translate(${mousePosition.x * 0.05}px, ${mousePosition.y * 0.05}px)`,
              }}
            >
              Nutreek
            </h1>

            <p
              className="text-lg text-slate-700 mb-8 max-w-2xl mx-auto backdrop-blur-sm bg-white/30 rounded-2xl p-4 border border-slate-200/50"
              style={{
                transform: `translate(${mousePosition.x * 0.03}px, ${mousePosition.y * 0.03}px)`,
              }}
            >
              Your intelligent nutrition planning companion for healthier, happier households
            </p>

            <div
              className="flex flex-col sm:flex-row gap-4 justify-center items-center"
              style={{
                transform: `translate(${mousePosition.x * 0.02}px, ${mousePosition.y * 0.02}px)`,
              }}
            >
              <Link
                href="/auth/login"
                className="bg-gradient-to-r from-primary to-accent hover:from-primary/90 hover:to-accent/90 text-white font-semibold py-4 px-8 rounded-2xl transition-all duration-300 flex items-center gap-2 group shadow-lg hover:shadow-xl backdrop-blur-sm border border-white/10"
              >
                Get Started
                <ArrowRight className="w-5 h-5 group-hover:translate-x-1 transition-transform" />
              </Link>
              <Link
                href="/auth/register"
                className="bg-white/30 hover:bg-white/40 text-slate-800 font-semibold py-4 px-8 rounded-2xl transition-all duration-300 flex items-center gap-2 backdrop-blur-sm border border-white/30 hover:border-white/40"
              >
                <Sparkles className="w-5 h-5" />
                Create Account
              </Link>
            </div>
          </div>
        </div>
      </section>

      {/* Features Section */}
      <section className="relative py-20 px-4 sm:px-6 lg:px-8">
        <div className="max-w-7xl mx-auto">
          <div
            className="text-center mb-16"
            style={{
              transform: `translate(${mousePosition.x * 0.01}px, ${mousePosition.y * 0.01}px)`,
            }}
          >
            <h2 className="text-heading-1 text-slate-800 mb-4 drop-shadow-lg">
              Why Choose Nutreek?
            </h2>
            <p className="text-body-large text-slate-700 max-w-2xl mx-auto backdrop-blur-sm bg-white/30 rounded-xl p-4 border border-slate-200/50">
              Everything you need for smart nutrition planning in one beautiful, easy-to-use platform
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
            <div
              className="bg-slate-100/40 hover:bg-slate-200/50 backdrop-blur-md rounded-3xl p-8 border border-slate-300/30 hover:border-slate-400/40 transition-all duration-300 group hover:scale-105 hover:shadow-2xl"
              style={{
                transform: `translate(${mousePosition.x * 0.02}px, ${mousePosition.y * 0.02}px)`,
              }}
            >
              <div className="flex items-center gap-4 mb-6">
                <div className="w-14 h-14 bg-gradient-to-br from-emerald-400/40 to-teal-500/40 rounded-2xl flex items-center justify-center group-hover:from-emerald-400/50 group-hover:to-teal-500/50 transition-all duration-300 backdrop-blur-sm border border-slate-300/30">
                  <Users className="w-7 h-7 text-emerald-600" />
                </div>
                <h3 className="text-xl font-semibold text-slate-800">
                  Multi-tenant households
                </h3>
              </div>
              <p className="text-slate-700 leading-relaxed">
                Manage nutrition planning for your entire household with shared meal plans and collaborative features.
              </p>
            </div>

            <div
              className="bg-slate-100/40 hover:bg-slate-200/50 backdrop-blur-md rounded-3xl p-8 border border-slate-300/30 hover:border-slate-400/40 transition-all duration-300 group hover:scale-105 hover:shadow-2xl"
              style={{
                transform: `translate(${mousePosition.x * -0.01}px, ${mousePosition.y * -0.01}px)`,
              }}
            >
              <div className="flex items-center gap-4 mb-6">
                <div className="w-14 h-14 bg-gradient-to-br from-blue-400/40 to-cyan-500/40 rounded-2xl flex items-center justify-center group-hover:from-blue-400/50 group-hover:to-cyan-500/50 transition-all duration-300 backdrop-blur-sm border border-slate-300/30">
                  <Calendar className="w-7 h-7 text-blue-600" />
                </div>
                <h3 className="text-xl font-semibold text-slate-800">
                  Weekly meal planning
                </h3>
              </div>
              <p className="text-slate-700 leading-relaxed">
                Plan your meals for the entire week with our intuitive planner interface and smart suggestions.
              </p>
            </div>

            <div
              className="bg-slate-100/40 hover:bg-slate-200/50 backdrop-blur-md rounded-3xl p-8 border border-slate-300/30 hover:border-slate-400/40 transition-all duration-300 group hover:scale-105 hover:shadow-2xl"
              style={{
                transform: `translate(${mousePosition.x * 0.015}px, ${mousePosition.y * 0.015}px)`,
              }}
            >
              <div className="flex items-center gap-4 mb-6">
                <div className="w-14 h-14 bg-gradient-to-br from-pink-400/40 to-rose-500/40 rounded-2xl flex items-center justify-center group-hover:from-pink-400/50 group-hover:to-rose-500/50 transition-all duration-300 backdrop-blur-sm border border-slate-300/30">
                  <ShoppingCart className="w-7 h-7 text-pink-600" />
                </div>
                <h3 className="text-xl font-semibold text-slate-800">
                  Smart shopping lists
                </h3>
              </div>
              <p className="text-slate-700 leading-relaxed">
                Automatically generate shopping lists based on your weekly meal plan with smart categorization.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* CTA Section */}
      <section className="relative py-20 px-4 sm:px-6 lg:px-8">
        <div className="max-w-4xl mx-auto text-center">
          <div
            className="bg-slate-100/30 hover:bg-slate-200/40 backdrop-blur-md rounded-3xl p-8 md:p-12 border border-slate-300/30 hover:border-slate-400/40 transition-all duration-300"
            style={{
              transform: `translate(${mousePosition.x * 0.005}px, ${mousePosition.y * 0.005}px)`,
            }}
          >
            <h2 className="text-heading-2 text-slate-800 mb-4 drop-shadow-lg">
              Ready to transform your nutrition planning?
            </h2>
            <p className="text-body-large text-slate-700 mb-8">
              Join thousands of households who have simplified their meal planning with Nutreek
            </p>
            <div className="flex flex-col sm:flex-row gap-4 justify-center">
              <Link
                href="/auth/login"
                className="bg-gradient-to-r from-primary to-accent hover:from-primary/90 hover:to-accent/90 text-white font-semibold py-4 px-8 rounded-2xl transition-all duration-300 flex items-center gap-2 group shadow-lg hover:shadow-xl"
              >
                Start Planning Today
                <ArrowRight className="w-5 h-5 group-hover:translate-x-1 transition-transform" />
              </Link>
            </div>
          </div>
        </div>
      </section>
    </main>
  )
}