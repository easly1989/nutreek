'use client';

import { useState, useRef, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { motion, AnimatePresence } from 'framer-motion';
import {
  ArrowRight,
  Plus,
  Search,
  Trash2,
  Sparkles,
  AlertCircle,
  CheckCircle,
  X,
  Target,
  Zap,
  Apple,
  Leaf,
  Loader2
} from 'lucide-react';
import apiClient from '../lib/api-client';
import IngredientSearch from './IngredientSearch';

interface Substitution {
  id: string;
  originalId: string;
  substituteId: string;
  user: {
    id: string;
    name?: string;
    email: string;
  };
  original?: {
    id: string;
    name: string;
    macros?: {
      kcal: number;
      protein: number;
      carbs: number;
      fat: number;
    };
  };
  substitute?: {
    id: string;
    name: string;
    macros?: {
      kcal: number;
      protein: number;
      carbs: number;
      fat: number;
    };
  };
}

interface SubstitutionModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSubstitutionCreated?: (substitution: Substitution) => void;
}

function SubstitutionModal({ isOpen, onClose, onSubstitutionCreated }: SubstitutionModalProps) {
  const [originalIngredient, setOriginalIngredient] = useState<any>(null);
  const [substituteIngredient, setSubstituteIngredient] = useState<any>(null);
  const [step, setStep] = useState<'select-original' | 'select-substitute' | 'compare' | 'confirm'>('select-original');
  const queryClient = useQueryClient();

  const createSubstitution = useMutation({
    mutationFn: async (data: { originalId: string; substituteId: string }) => {
      const response = await apiClient.post('/users/substitutions', data);
      return response.data;
    },
    onSuccess: (newSubstitution) => {
      queryClient.invalidateQueries({ queryKey: ['substitutions'] });
      onSubstitutionCreated?.(newSubstitution);
      handleClose();
    },
  });

  const handleClose = () => {
    setOriginalIngredient(null);
    setSubstituteIngredient(null);
    setStep('select-original');
    onClose();
  };

  const handleOriginalSelect = (ingredient: any) => {
    setOriginalIngredient(ingredient);
    setStep('select-substitute');
  };

  const handleSubstituteSelect = (ingredient: any) => {
    setSubstituteIngredient(ingredient);
    setStep('compare');
  };

  const handleConfirm = () => {
    if (!originalIngredient || !substituteIngredient) return;

    createSubstitution.mutate({
      originalId: originalIngredient.id,
      substituteId: substituteIngredient.id,
    });
  };

  const getNutritionComparison = () => {
    if (!originalIngredient?.macros || !substituteIngredient?.macros) return null;

    const original = originalIngredient.macros;
    const substitute = substituteIngredient.macros;

    return {
      calories: { original: original.kcal, substitute: substitute.kcal },
      protein: { original: original.protein, substitute: substitute.protein },
      carbs: { original: original.carbs, substitute: substitute.carbs },
      fat: { original: original.fat, substitute: substitute.fat },
    };
  };

  const comparison = getNutritionComparison();

  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={(e) => e.target === e.currentTarget && handleClose()}
        >
          <motion.div
            initial={{ scale: 0.9, opacity: 0, y: 20 }}
            animate={{ scale: 1, opacity: 1, y: 0 }}
            exit={{ scale: 0.9, opacity: 0, y: 20 }}
            transition={{ type: "spring", stiffness: 300, damping: 30 }}
            className="relative w-full max-w-2xl max-h-[90vh] overflow-hidden bg-card rounded-2xl shadow-2xl border border-border"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Header */}
            <div className="flex items-center justify-between p-6 border-b border-border">
              <div className="flex items-center space-x-3">
                <motion.div
                  whileHover={{ rotate: 360 }}
                  transition={{ duration: 0.5 }}
                  className="w-10 h-10 bg-primary rounded-xl flex items-center justify-center"
                >
                  <ArrowRight className="w-5 h-5 text-primary-foreground" />
                </motion.div>
                <div>
                  <h2 className="text-heading-2">Create Substitution</h2>
                  <p className="text-caption text-muted-foreground">
                    {step === 'select-original' && 'Choose the ingredient to replace'}
                    {step === 'select-substitute' && 'Choose the substitute ingredient'}
                    {step === 'compare' && 'Review nutrition comparison'}
                    {step === 'confirm' && 'Confirm your substitution'}
                  </p>
                </div>
              </div>
              <motion.button
                whileHover={{ scale: 1.1 }}
                whileTap={{ scale: 0.9 }}
                onClick={handleClose}
                className="p-2 rounded-lg hover:bg-muted transition-colors duration-200"
              >
                <X className="w-5 h-5" />
              </motion.button>
            </div>

            {/* Progress Bar */}
            <div className="px-6 py-4 bg-muted/30">
              <div className="flex items-center justify-between mb-2">
                <span className="text-caption font-medium">Progress</span>
                <span className="text-caption text-muted-foreground">
                  Step {['select-original', 'select-substitute', 'compare', 'confirm'].indexOf(step) + 1} of 4
                </span>
              </div>
              <div className="w-full bg-muted rounded-full h-2">
                <motion.div
                  className="bg-primary h-2 rounded-full"
                  initial={{ width: 0 }}
                  animate={{
                    width: `${((['select-original', 'select-substitute', 'compare', 'confirm'].indexOf(step) + 1) / 4) * 100}%`
                  }}
                  transition={{ duration: 0.5 }}
                />
              </div>
            </div>

            {/* Content */}
            <div className="p-6 overflow-y-auto max-h-[calc(90vh-200px)]">
              <AnimatePresence mode="wait">
                {/* Step 1: Select Original */}
                {step === 'select-original' && (
                  <motion.div
                    key="select-original"
                    initial={{ opacity: 0, x: -20 }}
                    animate={{ opacity: 1, x: 0 }}
                    exit={{ opacity: 0, x: -20 }}
                    transition={{ duration: 0.3 }}
                    className="space-y-6"
                  >
                    <div className="text-center">
                      <motion.div
                        animate={{ scale: [1, 1.1, 1] }}
                        transition={{ duration: 2, repeat: Infinity }}
                        className="w-16 h-16 bg-nutrition-calories/10 rounded-full flex items-center justify-center mx-auto mb-4"
                      >
                        <Target className="w-8 h-8 text-nutrition-calories" />
                      </motion.div>
                      <h3 className="text-heading-3 mb-2">Select Original Ingredient</h3>
                      <p className="text-body text-muted-foreground">
                        Choose the ingredient you want to replace in your recipes
                      </p>
                    </div>
                    <IngredientSearch
                      placeholder="Search for the ingredient to replace..."
                      onSelect={handleOriginalSelect}
                    />
                  </motion.div>
                )}

                {/* Step 2: Select Substitute */}
                {step === 'select-substitute' && (
                  <motion.div
                    key="select-substitute"
                    initial={{ opacity: 0, x: 20 }}
                    animate={{ opacity: 1, x: 0 }}
                    exit={{ opacity: 0, x: 20 }}
                    transition={{ duration: 0.3 }}
                    className="space-y-6"
                  >
                    <div className="flex items-center space-x-4 mb-6">
                      <motion.button
                        whileHover={{ scale: 1.05 }}
                        whileTap={{ scale: 0.95 }}
                        onClick={() => setStep('select-original')}
                        className="p-2 rounded-lg hover:bg-muted transition-colors duration-200"
                      >
                        <ArrowRight className="w-5 h-5 rotate-180" />
                      </motion.button>
                      <div className="flex items-center space-x-3">
                        <motion.div
                          animate={{ rotate: 360 }}
                          transition={{ duration: 0.5 }}
                          className="w-10 h-10 bg-primary rounded-xl flex items-center justify-center"
                        >
                          <Sparkles className="w-5 h-5 text-primary-foreground" />
                        </motion.div>
                        <div>
                          <h3 className="text-heading-3">Original: {originalIngredient?.name}</h3>
                          <p className="text-caption text-muted-foreground">Now choose your substitute</p>
                        </div>
                      </div>
                    </div>
                    <IngredientSearch
                      placeholder="Search for substitute ingredient..."
                      onSelect={handleSubstituteSelect}
                    />
                  </motion.div>
                )}

                {/* Step 3: Compare Nutrition */}
                {step === 'compare' && comparison && (
                  <motion.div
                    key="compare"
                    initial={{ opacity: 0, scale: 0.95 }}
                    animate={{ opacity: 1, scale: 1 }}
                    exit={{ opacity: 0, scale: 0.95 }}
                    transition={{ duration: 0.3 }}
                    className="space-y-6"
                  >
                    <div className="flex items-center space-x-4 mb-6">
                      <motion.button
                        whileHover={{ scale: 1.05 }}
                        whileTap={{ scale: 0.95 }}
                        onClick={() => setStep('select-substitute')}
                        className="p-2 rounded-lg hover:bg-muted transition-colors duration-200"
                      >
                        <ArrowRight className="w-5 h-5 rotate-180" />
                      </motion.button>
                      <div>
                        <h3 className="text-heading-3">Nutrition Comparison</h3>
                        <p className="text-caption text-muted-foreground">Review the nutritional differences</p>
                      </div>
                    </div>

                    {/* Ingredient Cards */}
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                      <motion.div
                        initial={{ opacity: 0, x: -20 }}
                        animate={{ opacity: 1, x: 0 }}
                        transition={{ delay: 0.1 }}
                        className="card-nutrition p-6"
                      >
                        <div className="text-center mb-4">
                          <h4 className="font-semibold text-destructive mb-2">Original</h4>
                          <p className="text-body">{originalIngredient?.name}</p>
                        </div>
                        <div className="space-y-3">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-2">
                              <Zap className="w-4 h-4 text-nutrition-calories" />
                              <span className="text-caption">Calories</span>
                            </div>
                            <span className="font-semibold text-nutrition-calories">
                              {comparison.calories.original} kcal
                            </span>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-2">
                              <Target className="w-4 h-4 text-nutrition-protein" />
                              <span className="text-caption">Protein</span>
                            </div>
                            <span className="font-semibold text-nutrition-protein">
                              {comparison.protein.original}g
                            </span>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-2">
                              <Apple className="w-4 h-4 text-nutrition-carbs" />
                              <span className="text-caption">Carbs</span>
                            </div>
                            <span className="font-semibold text-nutrition-carbs">
                              {comparison.carbs.original}g
                            </span>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-2">
                              <Leaf className="w-4 h-4 text-nutrition-fat" />
                              <span className="text-caption">Fat</span>
                            </div>
                            <span className="font-semibold text-nutrition-fat">
                              {comparison.fat.original}g
                            </span>
                          </div>
                        </div>
                      </motion.div>

                      <motion.div
                        initial={{ opacity: 0, x: 20 }}
                        animate={{ opacity: 1, x: 0 }}
                        transition={{ delay: 0.2 }}
                        className="card-nutrition p-6"
                      >
                        <div className="text-center mb-4">
                          <h4 className="font-semibold text-primary mb-2">Substitute</h4>
                          <p className="text-body">{substituteIngredient?.name}</p>
                        </div>
                        <div className="space-y-3">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-2">
                              <Zap className="w-4 h-4 text-nutrition-calories" />
                              <span className="text-caption">Calories</span>
                            </div>
                            <span className="font-semibold text-nutrition-calories">
                              {comparison.calories.substitute} kcal
                            </span>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-2">
                              <Target className="w-4 h-4 text-nutrition-protein" />
                              <span className="text-caption">Protein</span>
                            </div>
                            <span className="font-semibold text-nutrition-protein">
                              {comparison.protein.substitute}g
                            </span>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-2">
                              <Apple className="w-4 h-4 text-nutrition-carbs" />
                              <span className="text-caption">Carbs</span>
                            </div>
                            <span className="font-semibold text-nutrition-carbs">
                              {comparison.carbs.substitute}g
                            </span>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-2">
                              <Leaf className="w-4 h-4 text-nutrition-fat" />
                              <span className="text-caption">Fat</span>
                            </div>
                            <span className="font-semibold text-nutrition-fat">
                              {comparison.fat.substitute}g
                            </span>
                          </div>
                        </div>
                      </motion.div>
                    </div>
                  </motion.div>
                )}
              </AnimatePresence>
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between p-6 border-t border-border bg-muted/30">
              <motion.button
                whileHover={{ scale: 1.05 }}
                whileTap={{ scale: 0.95 }}
                onClick={handleClose}
                className="btn-nutrition-outline"
              >
                Cancel
              </motion.button>

              <div className="flex items-center space-x-3">
                {step === 'select-original' && originalIngredient && (
                  <motion.button
                    initial={{ opacity: 0, scale: 0.8 }}
                    animate={{ opacity: 1, scale: 1 }}
                    whileHover={{ scale: 1.05 }}
                    whileTap={{ scale: 0.95 }}
                    onClick={() => setStep('select-substitute')}
                    className="btn-nutrition"
                  >
                    Next Step
                    <ArrowRight className="w-4 h-4 ml-2" />
                  </motion.button>
                )}

                {step === 'compare' && (
                  <motion.button
                    initial={{ opacity: 0, scale: 0.8 }}
                    animate={{ opacity: 1, scale: 1 }}
                    whileHover={{ scale: 1.05 }}
                    whileTap={{ scale: 0.95 }}
                    onClick={handleConfirm}
                    disabled={createSubstitution.isPending}
                    className="btn-nutrition"
                  >
                    {createSubstitution.isPending ? (
                      <>
                        <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                        Creating...
                      </>
                    ) : (
                      <>
                        Create Substitution
                        <CheckCircle className="w-4 h-4 ml-2" />
                      </>
                    )}
                  </motion.button>
                )}
              </div>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

export default function SubstitutionManager() {
  const [showModal, setShowModal] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const queryClient = useQueryClient();

  // Fetch user's substitutions
  const { data: substitutions, isLoading } = useQuery({
    queryKey: ['substitutions'],
    queryFn: async () => {
      const response = await apiClient.get<Substitution[]>('/users/substitutions');
      return response.data;
    },
  });

  // Delete substitution
  const deleteSubstitution = useMutation({
    mutationFn: async (id: string) => {
      await apiClient.delete(`/users/substitutions/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['substitutions'] });
      setDeleteConfirm(null);
    },
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16">
        <motion.div
          animate={{ rotate: 360 }}
          transition={{ duration: 2, repeat: Infinity, ease: "linear" }}
          className="w-12 h-12 border-4 border-primary border-t-transparent rounded-full"
        />
        <motion.span
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.2 }}
          className="ml-3 text-body"
        >
          Loading your substitutions...
        </motion.span>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4"
      >
        <div>
          <h1 className="text-display bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
            Ingredient Substitutions
          </h1>
          <p className="text-body-large text-muted-foreground mt-1">
            Customize your recipes with healthy alternatives ✨
          </p>
        </div>
        <motion.button
          whileHover={{ scale: 1.05 }}
          whileTap={{ scale: 0.95 }}
          onClick={() => setShowModal(true)}
          className="btn-nutrition whitespace-nowrap"
        >
          <Plus className="w-4 h-4 mr-2" />
          Add Substitution
        </motion.button>
      </motion.div>

      {/* Substitutions Grid */}
      {substitutions && substitutions.length > 0 ? (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6"
        >
          {substitutions.map((substitution, index) => (
            <motion.div
              key={substitution.id}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: index * 0.1, duration: 0.5 }}
              whileHover={{ scale: 1.02, y: -2 }}
              className="card-nutrition hover-lift group"
            >
              <div className="p-6">
                {/* Header */}
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center space-x-2">
                    <motion.div
                      whileHover={{ rotate: 360 }}
                      transition={{ duration: 0.5 }}
                      className="w-8 h-8 bg-gradient-nutrition rounded-lg flex items-center justify-center"
                    >
                      <ArrowRight className="w-4 h-4 text-white" />
                    </motion.div>
                    <span className="text-caption font-medium text-primary">Substitution</span>
                  </div>
                  <motion.button
                    whileHover={{ scale: 1.1 }}
                    whileTap={{ scale: 0.9 }}
                    onClick={() => setDeleteConfirm(substitution.id)}
                    className="opacity-0 group-hover:opacity-100 p-2 rounded-lg hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-all duration-200"
                  >
                    <Trash2 className="w-4 h-4" />
                  </motion.button>
                </div>

                {/* Ingredient Comparison */}
                <div className="space-y-4">
                  <div className="flex items-center space-x-3 p-3 bg-muted/50 rounded-lg">
                    <div className="w-2 h-2 bg-destructive rounded-full"></div>
                    <span className="text-body line-through text-muted-foreground">
                      {substitution.original?.name || 'Unknown Ingredient'}
                    </span>
                  </div>

                  <motion.div
                    initial={{ scale: 0.95 }}
                    animate={{ scale: 1 }}
                    transition={{ delay: 0.2 }}
                    className="flex items-center space-x-3 p-3 bg-primary/10 rounded-lg"
                  >
                    <div className="w-2 h-2 bg-primary rounded-full"></div>
                    <span className="text-body font-semibold text-primary">
                      {substitution.substitute?.name || 'Unknown Substitute'}
                    </span>
                  </motion.div>
                </div>

                {/* Nutrition Comparison */}
                {substitution.original?.macros && substitution.substitute?.macros && (
                  <motion.div
                    initial={{ opacity: 0, height: 0 }}
                    animate={{ opacity: 1, height: 'auto' }}
                    transition={{ delay: 0.3 }}
                    className="mt-4 pt-4 border-t border-border"
                  >
                    <h4 className="text-caption font-medium mb-3">Nutrition Comparison</h4>
                    <div className="grid grid-cols-2 gap-3 text-xs">
                      <div className="flex justify-between">
                        <span className="text-nutrition-calories">Calories</span>
                        <span>{substitution.original.macros.kcal} → {substitution.substitute.macros.kcal}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-nutrition-protein">Protein</span>
                        <span>{substitution.original.macros.protein}g → {substitution.substitute.macros.protein}g</span>
                      </div>
                    </div>
                  </motion.div>
                )}
              </div>
            </motion.div>
          ))}
        </motion.div>
      ) : (
        /* Empty State */
        <motion.div
          initial={{ opacity: 0, scale: 0.9 }}
          animate={{ opacity: 1, scale: 1 }}
          className="text-center py-16"
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
            className="w-24 h-24 mx-auto mb-6 bg-gradient-nutrition rounded-2xl flex items-center justify-center"
          >
            <ArrowRight className="w-12 h-12 text-white" />
          </motion.div>
          <h3 className="text-heading-2 mb-2">No substitutions yet</h3>
          <p className="text-body-large text-muted-foreground mb-8 max-w-md mx-auto">
            Create ingredient substitutions to customize your recipes with healthier alternatives or dietary preferences.
          </p>
          <motion.button
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            onClick={() => setShowModal(true)}
            className="btn-nutrition px-8 py-4"
          >
            <Plus className="w-5 h-5 mr-2" />
            Create Your First Substitution
          </motion.button>
        </motion.div>
      )}

      {/* Substitution Modal */}
      <SubstitutionModal
        isOpen={showModal}
        onClose={() => setShowModal(false)}
      />

      {/* Delete Confirmation */}
      <AnimatePresence>
        {deleteConfirm && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
            onClick={() => setDeleteConfirm(null)}
          >
            <motion.div
              initial={{ scale: 0.9, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ scale: 0.9, opacity: 0 }}
              className="bg-card p-6 rounded-xl shadow-xl border border-border max-w-md w-full"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="text-center">
                <motion.div
                  animate={{ scale: [1, 1.2, 1] }}
                  transition={{ duration: 0.5 }}
                  className="w-12 h-12 bg-destructive/10 rounded-full flex items-center justify-center mx-auto mb-4"
                >
                  <AlertCircle className="w-6 h-6 text-destructive" />
                </motion.div>
                <h3 className="text-heading-3 mb-2">Delete Substitution</h3>
                <p className="text-body text-muted-foreground mb-6">
                  Are you sure you want to delete this substitution? This action cannot be undone.
                </p>
                <div className="flex space-x-3">
                  <motion.button
                    whileHover={{ scale: 1.05 }}
                    whileTap={{ scale: 0.95 }}
                    onClick={() => setDeleteConfirm(null)}
                    className="flex-1 btn-nutrition-outline"
                  >
                    Cancel
                  </motion.button>
                  <motion.button
                    whileHover={{ scale: 1.05 }}
                    whileTap={{ scale: 0.95 }}
                    onClick={() => deleteSubstitution.mutate(deleteConfirm)}
                    disabled={deleteSubstitution.isPending}
                    className="flex-1 bg-destructive text-destructive-foreground hover:bg-destructive/90 btn-nutrition"
                  >
                    {deleteSubstitution.isPending ? 'Deleting...' : 'Delete'}
                  </motion.button>
                </div>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}