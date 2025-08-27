'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { motion } from 'framer-motion';
import { useLogin } from '../../../hooks/use-auth';
import {
  Mail,
  Sparkles,
  ChefHat,
  AlertCircle,
  Loader2,
  ArrowRight
} from 'lucide-react';

export default function LoginPage() {
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const [mousePosition, setMousePosition] = useState({ x: 0, y: 0 });
  const [scrollPosition, setScrollPosition] = useState(0);
  const [hoveredIcon, setHoveredIcon] = useState<number | null>(null);
  const router = useRouter();
  const login = useLogin();

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

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    try {
      await login.mutateAsync({ email });
      router.push('/dashboard');
    } catch (err: any) {
      setError(err.response?.data?.message || 'Login failed');
    }
  };

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

      <div className="min-h-screen flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8 overflow-hidden relative z-10">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="max-w-md w-full space-y-8"
        >
          {/* Header */}
          <motion.div
            initial={{ opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            transition={{ delay: 0.1, duration: 0.5 }}
            className="text-center"
          >
            <motion.div
              animate={{
                rotate: [0, 10, -10, 0],
                scale: [1, 1.1, 1]
              }}
              transition={{
                duration: 2,
                repeat: Infinity,
                ease: "easeInOut"
              }}
              className="w-20 h-20 bg-white/20 rounded-2xl flex items-center justify-center mx-auto mb-6 backdrop-blur-sm"
            >
              <ChefHat className="w-10 h-10 text-primary" />
            </motion.div>
            <h1 className="text-display bg-gradient-to-r from-slate-800 to-slate-600 bg-clip-text text-transparent">
              Welcome Back
            </h1>
            <p className="text-body text-slate-700 mt-2">
              Continue your nutrition journey! 🥗✨
            </p>
          </motion.div>

          {/* Form */}
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.2, duration: 0.5 }}
            className="bg-white/40 backdrop-blur-md rounded-2xl p-8 shadow-2xl border border-white/50"
          >
            <form className="space-y-6" onSubmit={handleSubmit}>
              {/* Error Message */}
              {error && (
                <motion.div
                  initial={{ opacity: 0, scale: 0.95 }}
                  animate={{ opacity: 1, scale: 1 }}
                  className="bg-red-50 border border-red-200 rounded-xl p-4"
                >
                  <div className="flex items-center space-x-2 text-red-600">
                    <AlertCircle className="w-5 h-5" />
                    <span className="text-sm font-medium">{error}</span>
                  </div>
                </motion.div>
              )}

              {/* Email Input */}
              <motion.div
                initial={{ opacity: 0, x: -20 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ delay: 0.3, duration: 0.5 }}
                className="space-y-2"
              >
                <label htmlFor="email" className="block text-sm font-medium text-slate-700">
                  Email Address
                </label>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                    <Mail className="w-5 h-5 text-slate-500" />
                  </div>
                  <input
                    id="email"
                    name="email"
                    type="email"
                    autoComplete="email"
                    required
                    className="focus-visible w-full pl-12 pr-4 py-4 bg-white/80 border border-slate-300 rounded-xl text-slate-800 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary/50 transition-all duration-200 backdrop-blur-sm"
                    placeholder="Enter your email address"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                  />
                </div>
              </motion.div>

              {/* Submit Button */}
              <motion.button
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.4, duration: 0.5 }}
                whileHover={{ scale: 1.02 }}
                whileTap={{ scale: 0.98 }}
                type="submit"
                disabled={login.isPending}
                className="w-full bg-primary text-white hover:bg-primary/90 focus:outline-none focus:ring-2 focus:ring-primary/50 font-semibold py-4 px-6 rounded-xl transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center space-x-2"
              >
                {login.isPending ? (
                  <>
                    <Loader2 className="w-5 h-5 animate-spin" />
                    <span>Signing in...</span>
                  </>
                ) : (
                  <>
                    <Sparkles className="w-5 h-5" />
                    <span>Sign In</span>
                  </>
                )}
              </motion.button>

              {/* Register Link */}
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 0.5, duration: 0.5 }}
                className="text-center"
              >
                <p className="text-slate-600">
                  New to Nutreek?{' '}
                  <Link
                    href="/auth/register"
                    className="font-semibold text-primary hover:text-primary/80 transition-colors duration-200 underline decoration-primary/30 hover:decoration-primary/50 flex items-center justify-center mt-2"
                  >
                    Create Account
                    <ArrowRight className="w-4 h-4 ml-1" />
                  </Link>
                </p>
              </motion.div>
            </form>
          </motion.div>

          {/* Footer */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.6, duration: 0.5 }}
            className="text-center text-slate-500 text-sm"
          >
            <Link href="/forgot-password" className="hover:text-slate-700 underline">
              Forgot your password?
            </Link>
          </motion.div>
        </motion.div>
      </div>
    </>
  );
}