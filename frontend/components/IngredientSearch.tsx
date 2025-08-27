'use client';

import { useState, useRef, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { motion, AnimatePresence } from 'framer-motion';
import {
  Search,
  Loader2,
  Sparkles,
  Target,
  Zap,
  Apple,
  Leaf,
  AlertCircle,
  CheckCircle,
  Plus
} from 'lucide-react';
import apiClient from '../lib/api-client';

interface Ingredient {
  id: string;
  name: string;
  unit: string;
  amount: number;
  macros?: {
    kcal: number;
    protein: number;
    carbs: number;
    fat: number;
    fiber?: number;
    sugar?: number;
  };
  source?: string;
}

interface IngredientSearchProps {
  onSelect?: (ingredient: Ingredient) => void;
  placeholder?: string;
  showDetails?: boolean;
  className?: string;
}

export default function IngredientSearch({
  onSelect,
  placeholder = "Search for nutritious ingredients...",
  showDetails = true,
  className = ""
}: IngredientSearchProps) {
  const [query, setQuery] = useState('');
  const [selectedIngredient, setSelectedIngredient] = useState<Ingredient | null>(null);
  const [isFocused, setIsFocused] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const { data: ingredients, isLoading, error } = useQuery({
    queryKey: ['ingredients-search', query],
    queryFn: async () => {
      if (!query.trim()) return [];
      const response = await apiClient.get<Ingredient[]>(`/ingredients/search?q=${encodeURIComponent(query)}`);
      return response.data;
    },
    enabled: query.length > 2,
  });

  const handleIngredientClick = (ingredient: Ingredient) => {
    setSelectedIngredient(ingredient);
    setQuery('');
    setIsFocused(false);
    setSelectedIndex(-1);
    onSelect?.(ingredient);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!ingredients || ingredients.length === 0) return;

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setSelectedIndex(prev =>
          prev < ingredients.length - 1 ? prev + 1 : prev
        );
        break;
      case 'ArrowUp':
        e.preventDefault();
        setSelectedIndex(prev => prev > 0 ? prev - 1 : -1);
        break;
      case 'Enter':
        e.preventDefault();
        if (selectedIndex >= 0 && ingredients[selectedIndex]) {
          handleIngredientClick(ingredients[selectedIndex]);
        }
        break;
      case 'Escape':
        setIsFocused(false);
        setSelectedIndex(-1);
        inputRef.current?.blur();
        break;
    }
  };

  const getNutritionColor = (value: number, type: string) => {
    switch (type) {
      case 'protein':
        return value > 20 ? 'text-nutrition-protein' : 'text-muted-foreground';
      case 'carbs':
        return value > 50 ? 'text-nutrition-carbs' : 'text-muted-foreground';
      case 'fat':
        return value > 20 ? 'text-nutrition-fat' : 'text-muted-foreground';
      case 'calories':
        return value > 300 ? 'text-nutrition-calories' : 'text-muted-foreground';
      default:
        return 'text-muted-foreground';
    }
  };

  const getSourceIcon = (source?: string) => {
    switch (source) {
      case 'fatsecret':
        return <Target className="w-3 h-3" />;
      case 'usda':
        return <Apple className="w-3 h-3" />;
      default:
        return <Leaf className="w-3 h-3" />;
    }
  };

  const getSourceColor = (source?: string) => {
    switch (source) {
      case 'fatsecret':
        return 'bg-blue-100 text-blue-700';
      case 'usda':
        return 'bg-green-100 text-green-700';
      default:
        return 'bg-purple-100 text-purple-700';
    }
  };

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node) &&
          inputRef.current && !inputRef.current.contains(event.target as Node)) {
        setIsFocused(false);
        setSelectedIndex(-1);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  return (
    <div className={`relative ${className}`}>
      {/* Search Input */}
      <motion.div
        animate={{
          scale: isFocused ? 1.02 : 1,
          boxShadow: isFocused
            ? '0 10px 25px -5px rgba(76, 175, 80, 0.2), 0 4px 6px -2px rgba(76, 175, 80, 0.1)'
            : '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)'
        }}
        transition={{ duration: 0.2 }}
        className="relative z-10"
      >
        <div className="relative">
          <Search className="absolute left-4 top-1/2 transform -translate-y-1/2 w-5 h-5 text-muted-foreground" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onFocus={() => setIsFocused(true)}
            onKeyDown={handleKeyDown}
            placeholder={placeholder}
            className="w-full pl-12 pr-12 py-4 bg-card border border-border rounded-full focus:outline-none focus:ring-0 transition-all duration-200 text-body"
          />
          <div className="absolute right-4 top-1/2 transform -translate-y-1/2 flex items-center space-x-2">
            {isLoading && query.length > 2 && (
              <motion.div
                animate={{ rotate: 360 }}
                transition={{ duration: 1, repeat: Infinity, ease: "linear" }}
              >
                <Loader2 className="w-5 h-5 text-primary" />
              </motion.div>
            )}
            {query && (
              <motion.button
                whileHover={{ scale: 1.1 }}
                whileTap={{ scale: 0.9 }}
                onClick={() => {
                  setQuery('');
                  setSelectedIndex(-1);
                  inputRef.current?.focus();
                }}
                className="p-1 rounded-lg hover:bg-muted transition-colors duration-200"
              >
                <svg className="w-4 h-4 text-muted-foreground" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </motion.button>
            )}
          </div>
        </div>

        {/* Helper text */}
        <AnimatePresence>
          {query.length > 0 && query.length <= 2 && (
            <motion.p
              initial={{ opacity: 0, y: -10 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
              className="text-caption text-muted-foreground mt-2 flex items-center"
            >
              <Sparkles className="w-3 h-3 mr-1" />
              Type at least 3 characters to search
            </motion.p>
          )}
        </AnimatePresence>
      </motion.div>

      {/* Dropdown Results */}
      <AnimatePresence>
        {(ingredients && ingredients.length > 0 && (isFocused || query.length > 2)) && (
          <motion.div
            ref={dropdownRef}
            initial={{ opacity: 0, y: -10, scale: 0.95 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -10, scale: 0.95 }}
            transition={{ duration: 0.2 }}
            className="absolute z-50 w-full mt-3 bg-card border border-border rounded-xl shadow-xl overflow-hidden"
          >
            <div className="max-h-80 overflow-y-auto">
              {ingredients.map((ingredient, index) => (
                <motion.button
                  key={ingredient.id}
                  initial={{ opacity: 0, x: -20 }}
                  animate={{ opacity: 1, x: 0 }}
                  transition={{ delay: index * 0.05, duration: 0.3 }}
                  whileHover={{
                    scale: 1.05,
                    backgroundColor: 'hsl(var(--muted))',
                    boxShadow: '0 10px 25px -5px rgba(76, 175, 80, 0.2), 0 4px 6px -2px rgba(76, 175, 80, 0.1)'
                  }}
                  whileTap={{ scale: 0.98 }}
                  onClick={() => handleIngredientClick(ingredient)}
                  className={`w-full px-4 py-3 text-left border-b border-border/50 last:border-b-0 transition-all duration-200 ${
                    selectedIndex === index ? 'bg-primary/10' : ''
                  }`}
                >
                  <div className="flex items-start justify-between">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center space-x-2 mb-1">
                        <h4 className="font-semibold text-foreground truncate">{ingredient.name}</h4>
                        {ingredient.source && (
                          <span className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${getSourceColor(ingredient.source)}`}>
                            {getSourceIcon(ingredient.source)}
                            <span className="ml-1 capitalize">{ingredient.source}</span>
                          </span>
                        )}
                      </div>

                      {showDetails && ingredient.macros && (
                        <div className="flex items-center space-x-4 text-caption">
                          <div className="flex items-center space-x-1">
                            <Zap className="w-3 h-3 text-nutrition-calories" />
                            <span className={getNutritionColor(ingredient.macros.kcal, 'calories')}>
                              {Math.round(ingredient.macros.kcal)} kcal
                            </span>
                          </div>
                          <div className="flex items-center space-x-1">
                            <Target className="w-3 h-3 text-nutrition-protein" />
                            <span className={getNutritionColor(ingredient.macros.protein, 'protein')}>
                              {Math.round(ingredient.macros.protein)}g protein
                            </span>
                          </div>
                          <div className="flex items-center space-x-1">
                            <Apple className="w-3 h-3 text-nutrition-carbs" />
                            <span className={getNutritionColor(ingredient.macros.carbs, 'carbs')}>
                              {Math.round(ingredient.macros.carbs)}g carbs
                            </span>
                          </div>
                          <div className="flex items-center space-x-1">
                            <Leaf className="w-3 h-3 text-nutrition-fat" />
                            <span className={getNutritionColor(ingredient.macros.fat, 'fat')}>
                              {Math.round(ingredient.macros.fat)}g fat
                            </span>
                          </div>
                        </div>
                      )}

                      <div className="text-caption text-muted-foreground mt-1">
                        Serving: {ingredient.amount} {ingredient.unit}
                      </div>
                    </div>

                    <motion.div
                      whileHover={{ scale: 1.1 }}
                      className="ml-3 p-2 rounded-lg bg-primary/10 text-primary opacity-0 group-hover:opacity-100 transition-opacity duration-200"
                    >
                      <Plus className="w-4 h-4" />
                    </motion.div>
                  </div>
                </motion.button>
              ))}
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Error State */}
      <AnimatePresence>
        {error && (
          <motion.div
            initial={{ opacity: 0, y: -10, scale: 0.95 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -10, scale: 0.95 }}
            className="absolute z-50 w-full mt-3 bg-destructive/10 border border-destructive/20 rounded-xl p-4"
          >
            <div className="flex items-center space-x-2 text-destructive">
              <AlertCircle className="w-5 h-5" />
              <p className="text-sm font-medium">Search Error</p>
            </div>
            <p className="text-sm text-destructive/80 mt-1">
              Unable to search ingredients. Please try again.
            </p>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Selected Ingredient Details */}
      <AnimatePresence>
        {selectedIngredient && showDetails && (
          <motion.div
            initial={{ opacity: 0, y: 20, scale: 0.95 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 20, scale: 0.95 }}
            className="mt-4 card-nutrition p-6"
          >
            <div className="flex items-start justify-between mb-4">
              <div className="flex items-center space-x-2">
                <CheckCircle className="w-5 h-5 text-green-500" />
                <h4 className="font-semibold text-foreground">Selected Ingredient</h4>
              </div>
              {selectedIngredient.source && (
                <span className={`inline-flex items-center px-3 py-1 rounded-full text-sm font-medium ${getSourceColor(selectedIngredient.source)}`}>
                  {getSourceIcon(selectedIngredient.source)}
                  <span className="ml-1 capitalize">{selectedIngredient.source}</span>
                </span>
              )}
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              <div>
                <span className="text-caption text-muted-foreground">Name</span>
                <p className="font-semibold text-foreground mt-1">{selectedIngredient.name}</p>
              </div>
              <div>
                <span className="text-caption text-muted-foreground">Serving Size</span>
                <p className="font-semibold text-foreground mt-1">
                  {selectedIngredient.amount} {selectedIngredient.unit}
                </p>
              </div>
              {selectedIngredient.macros && (
                <>
                  <div>
                    <span className="text-caption text-muted-foreground">Calories</span>
                    <p className={`font-semibold mt-1 ${getNutritionColor(selectedIngredient.macros.kcal, 'calories')}`}>
                      {Math.round(selectedIngredient.macros.kcal)} kcal
                    </p>
                  </div>
                  <div>
                    <span className="text-caption text-muted-foreground">Protein</span>
                    <p className={`font-semibold mt-1 ${getNutritionColor(selectedIngredient.macros.protein, 'protein')}`}>
                      {Math.round(selectedIngredient.macros.protein)}g
                    </p>
                  </div>
                  <div>
                    <span className="text-caption text-muted-foreground">Carbohydrates</span>
                    <p className={`font-semibold mt-1 ${getNutritionColor(selectedIngredient.macros.carbs, 'carbs')}`}>
                      {Math.round(selectedIngredient.macros.carbs)}g
                    </p>
                  </div>
                  <div>
                    <span className="text-caption text-muted-foreground">Fat</span>
                    <p className={`font-semibold mt-1 ${getNutritionColor(selectedIngredient.macros.fat, 'fat')}`}>
                      {Math.round(selectedIngredient.macros.fat)}g
                    </p>
                  </div>
                </>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}