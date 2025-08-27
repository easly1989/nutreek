'use client';

import { useState, useRef, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { motion, AnimatePresence, Reorder } from 'framer-motion';
import {
  ShoppingCart,
  Plus,
  Check,
  ChevronDown,
  ChevronRight,
  Search,
  Trash2,
  Edit,
  Sparkles,
  Leaf,
  Beef,
  Milk,
  Wheat,
  Apple,
  X,
  CheckSquare,
  Square,
  GripVertical
} from 'lucide-react';
import apiClient from '../lib/api-client';

interface ShoppingListItem {
  id: string;
  name: string;
  category: string;
  quantity: string;
  unit: string;
  completed: boolean;
  recipe?: string;
  notes?: string;
}

interface ShoppingListProps {
  tenantId: string;
}

// Category configuration with icons and colors
const categories = [
  {
    id: 'produce',
    name: 'Produce',
    icon: Apple,
    color: 'bg-green-500',
    bgColor: 'bg-green-50 dark:bg-green-950/20',
    textColor: 'text-green-700 dark:text-green-300'
  },
  {
    id: 'protein',
    name: 'Protein',
    icon: Beef,
    color: 'bg-red-500',
    bgColor: 'bg-red-50 dark:bg-red-950/20',
    textColor: 'text-red-700 dark:text-red-300'
  },
  {
    id: 'dairy',
    name: 'Dairy',
    icon: Milk,
    color: 'bg-blue-500',
    bgColor: 'bg-blue-50 dark:bg-blue-950/20',
    textColor: 'text-blue-700 dark:text-blue-300'
  },
  {
    id: 'grains',
    name: 'Grains & Bread',
    icon: Wheat,
    color: 'bg-yellow-500',
    bgColor: 'bg-yellow-50 dark:bg-yellow-950/20',
    textColor: 'text-yellow-700 dark:text-yellow-300'
  },
  {
    id: 'pantry',
    name: 'Pantry Staples',
    icon: Leaf,
    color: 'bg-purple-500',
    bgColor: 'bg-purple-50 dark:bg-purple-950/20',
    textColor: 'text-purple-700 dark:text-purple-300'
  },
  {
    id: 'other',
    name: 'Other',
    icon: ShoppingCart,
    color: 'bg-gray-500',
    bgColor: 'bg-gray-50 dark:bg-gray-950/20',
    textColor: 'text-gray-700 dark:text-gray-300'
  }
];

function ShoppingListModal({ isOpen, onClose, onItemAdded }: {
  isOpen: boolean;
  onClose: () => void;
  onItemAdded: (item: ShoppingListItem) => void;
}) {
  const [itemData, setItemData] = useState({
    name: '',
    category: 'produce',
    quantity: '',
    unit: 'pieces',
    notes: ''
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!itemData.name.trim()) return;

    const newItem: ShoppingListItem = {
      id: Date.now().toString(),
      name: itemData.name,
      category: itemData.category,
      quantity: itemData.quantity,
      unit: itemData.unit,
      completed: false,
      notes: itemData.notes || undefined
    };

    onItemAdded(newItem);
    setItemData({ name: '', category: 'produce', quantity: '', unit: 'pieces', notes: '' });
  };

  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={(e) => e.target === e.currentTarget && onClose()}
        >
          <motion.div
            initial={{ scale: 0.9, opacity: 0, y: 20 }}
            animate={{ scale: 1, opacity: 1, y: 0 }}
            exit={{ scale: 0.9, opacity: 0, y: 20 }}
            className="bg-card p-6 rounded-2xl shadow-2xl border border-border w-full max-w-md"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-6">
              <div className="flex items-center space-x-3">
                <motion.div
                  whileHover={{ rotate: 360 }}
                  transition={{ duration: 0.5 }}
                  className="w-10 h-10 bg-primary rounded-xl flex items-center justify-center"
                >
                  <Plus className="w-5 h-5 text-primary-foreground" />
                </motion.div>
                <h3 className="text-heading-3">Add Item</h3>
              </div>
              <motion.button
                whileHover={{ scale: 1.1 }}
                whileTap={{ scale: 0.9 }}
                onClick={onClose}
                className="p-2 rounded-lg hover:bg-muted transition-colors duration-200"
              >
                <X className="w-5 h-5" />
              </motion.button>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-2">Item Name</label>
                <input
                  type="text"
                  value={itemData.name}
                  onChange={(e) => setItemData({...itemData, name: e.target.value})}
                  className="w-full px-4 py-3 bg-muted border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent transition-colors duration-200"
                  placeholder="e.g., Organic bananas"
                  required
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium mb-2">Quantity</label>
                  <input
                    type="text"
                    value={itemData.quantity}
                    onChange={(e) => setItemData({...itemData, quantity: e.target.value})}
                    className="w-full px-4 py-3 bg-muted border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent transition-colors duration-200"
                    placeholder="e.g., 6"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-2">Unit</label>
                  <select
                    value={itemData.unit}
                    onChange={(e) => setItemData({...itemData, unit: e.target.value})}
                    className="w-full px-4 py-3 bg-muted border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent transition-colors duration-200"
                  >
                    <option value="pieces">pieces</option>
                    <option value="lbs">lbs</option>
                    <option value="kg">kg</option>
                    <option value="oz">oz</option>
                    <option value="cups">cups</option>
                    <option value="bottles">bottles</option>
                    <option value="cans">cans</option>
                    <option value="packages">packages</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium mb-2">Category</label>
                <select
                  value={itemData.category}
                  onChange={(e) => setItemData({...itemData, category: e.target.value})}
                  className="w-full px-4 py-3 bg-muted border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent transition-colors duration-200"
                >
                  {categories.map(category => (
                    <option key={category.id} value={category.id}>
                      {category.name}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium mb-2">Notes (Optional)</label>
                <textarea
                  value={itemData.notes}
                  onChange={(e) => setItemData({...itemData, notes: e.target.value})}
                  className="w-full px-4 py-3 bg-muted border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent transition-colors duration-200 resize-none"
                  rows={3}
                  placeholder="Any special notes..."
                />
              </div>

              <div className="flex space-x-3 pt-4">
                <motion.button
                  whileHover={{ scale: 1.05 }}
                  whileTap={{ scale: 0.95 }}
                  type="button"
                  onClick={onClose}
                  className="flex-1 btn-nutrition-outline"
                >
                  Cancel
                </motion.button>
                <motion.button
                  whileHover={{ scale: 1.05 }}
                  whileTap={{ scale: 0.95 }}
                  type="submit"
                  className="flex-1 btn-nutrition"
                >
                  Add Item
                </motion.button>
              </div>
            </form>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

function ShoppingListItem({
  item,
  onToggle,
  onDelete,
  onEdit
}: {
  item: ShoppingListItem;
  onToggle: (id: string) => void;
  onDelete: (id: string) => void;
  onEdit: (item: ShoppingListItem) => void;
}) {
  return (
    <Reorder.Item
      value={item}
      dragListener={false}
      className="group"
    >
      <motion.div
        layout
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: -20, scale: 0.95 }}
        transition={{ duration: 0.3 }}
        className={`flex items-center space-x-4 p-4 rounded-xl border transition-all duration-300 ${
          item.completed
            ? 'bg-muted/50 border-muted-foreground/20'
            : 'bg-card border-border hover:border-primary/50 hover:shadow-md'
        }`}
      >
        {/* Drag Handle */}
        <motion.div
          whileHover={{ scale: 1.1 }}
          className="cursor-grab active:cursor-grabbing opacity-0 group-hover:opacity-100 transition-opacity duration-200"
        >
          <GripVertical className="w-4 h-4 text-muted-foreground" />
        </motion.div>

        {/* Checkbox */}
        <motion.button
          whileHover={{ scale: 1.1 }}
          whileTap={{ scale: 0.9 }}
          onClick={() => onToggle(item.id)}
          className={`flex-shrink-0 w-6 h-6 rounded-lg border-2 transition-all duration-300 ${
            item.completed
              ? 'bg-primary border-primary'
              : 'border-muted-foreground hover:border-primary'
          }`}
        >
          <AnimatePresence>
            {item.completed && (
              <motion.div
                initial={{ scale: 0, opacity: 0 }}
                animate={{ scale: 1, opacity: 1 }}
                exit={{ scale: 0, opacity: 0 }}
                transition={{ duration: 0.2 }}
              >
                <Check className="w-4 h-4 text-primary-foreground" />
              </motion.div>
            )}
          </AnimatePresence>
        </motion.button>

        {/* Item Content */}
        <div className="flex-1 min-w-0">
          <motion.div
            animate={{
              textDecoration: item.completed ? 'line-through' : 'none',
              opacity: item.completed ? 0.6 : 1
            }}
            transition={{ duration: 0.3 }}
          >
            <div className="flex items-center space-x-2">
              <span className="font-medium truncate">{item.name}</span>
              {(item.quantity || item.unit !== 'pieces') && (
                <span className="text-caption text-muted-foreground bg-muted px-2 py-1 rounded">
                  {item.quantity} {item.unit}
                </span>
              )}
            </div>
            {item.notes && (
              <p className="text-caption text-muted-foreground mt-1">{item.notes}</p>
            )}
          </motion.div>
        </div>

        {/* Actions */}
        <div className="flex items-center space-x-2 opacity-0 group-hover:opacity-100 transition-opacity duration-200">
          <motion.button
            whileHover={{ scale: 1.1 }}
            whileTap={{ scale: 0.9 }}
            onClick={() => onEdit(item)}
            className="p-2 rounded-lg hover:bg-muted transition-colors duration-200"
          >
            <Edit className="w-4 h-4" />
          </motion.button>
          <motion.button
            whileHover={{ scale: 1.1 }}
            whileTap={{ scale: 0.9 }}
            onClick={() => onDelete(item.id)}
            className="p-2 rounded-lg hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors duration-200"
          >
            <Trash2 className="w-4 h-4" />
          </motion.button>
        </div>
      </motion.div>
    </Reorder.Item>
  );
}

export default function ShoppingList({ tenantId }: ShoppingListProps) {
  const [items, setItems] = useState<ShoppingListItem[]>([
    // Mock data - in a real app this would come from the API
    { id: '1', name: 'Organic Bananas', category: 'produce', quantity: '6', unit: 'pieces', completed: false, recipe: 'Smoothie Bowl' },
    { id: '2', name: 'Chicken Breast', category: 'protein', quantity: '2', unit: 'lbs', completed: false, recipe: 'Grilled Chicken Salad' },
    { id: '3', name: 'Greek Yogurt', category: 'dairy', quantity: '1', unit: 'container', completed: true, recipe: 'Parfait' },
    { id: '4', name: 'Whole Wheat Bread', category: 'grains', quantity: '1', unit: 'loaf', completed: false, recipe: 'Turkey Sandwich' },
    { id: '5', name: 'Spinach', category: 'produce', quantity: '1', unit: 'bag', completed: false, recipe: 'Salad' },
    { id: '6', name: 'Salmon Fillet', category: 'protein', quantity: '4', unit: 'pieces', completed: false, recipe: 'Baked Salmon' },
  ]);
  const [showAddModal, setShowAddModal] = useState(false);
  const [expandedCategories, setExpandedCategories] = useState<Set<string>>(new Set(['produce', 'protein']));
  const [searchQuery, setSearchQuery] = useState('');
  const queryClient = useQueryClient();

  const toggleCategory = (categoryId: string) => {
    const newExpanded = new Set(expandedCategories);
    if (newExpanded.has(categoryId)) {
      newExpanded.delete(categoryId);
    } else {
      newExpanded.add(categoryId);
    }
    setExpandedCategories(newExpanded);
  };

  const toggleItem = (itemId: string) => {
    setItems(prev =>
      prev.map(item =>
        item.id === itemId ? { ...item, completed: !item.completed } : item
      )
    );
  };

  const deleteItem = (itemId: string) => {
    setItems(prev => prev.filter(item => item.id !== itemId));
  };

  const addItem = (newItem: ShoppingListItem) => {
    setItems(prev => [...prev, newItem]);
  };

  const editItem = (updatedItem: ShoppingListItem) => {
    setItems(prev =>
      prev.map(item =>
        item.id === updatedItem.id ? updatedItem : item
      )
    );
  };

  const filteredItems = items.filter(item =>
    item.name.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const itemsByCategory = categories.map(category => ({
    ...category,
    items: filteredItems.filter(item => item.category === category.id)
  }));

  const completedCount = items.filter(item => item.completed).length;
  const totalCount = items.length;

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
            Shopping List
          </h1>
          <p className="text-body-large text-muted-foreground mt-1">
            Organize your grocery shopping with smart categorization 🛒
          </p>
        </div>
        <motion.button
          whileHover={{ scale: 1.05 }}
          whileTap={{ scale: 0.95 }}
          onClick={() => setShowAddModal(true)}
          className="btn-nutrition whitespace-nowrap"
        >
          <Plus className="w-4 h-4 mr-2" />
          Add Item
        </motion.button>
      </motion.div>

      {/* Progress Overview */}
      <motion.div
        initial={{ opacity: 0, scale: 0.95 }}
        animate={{ opacity: 1, scale: 1 }}
        className="card-nutrition p-6"
      >
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-heading-3">Shopping Progress</h3>
          <span className="text-caption text-muted-foreground">
            {completedCount} of {totalCount} items completed
          </span>
        </div>
        <div className="w-full bg-muted rounded-full h-3 overflow-hidden">
          <motion.div
            className="bg-gradient-to-r from-primary to-accent h-full rounded-full"
            initial={{ width: 0 }}
            animate={{ width: `${(completedCount / totalCount) * 100}%` }}
            transition={{ duration: 1, ease: "easeOut" }}
          />
        </div>
        <p className="text-caption text-muted-foreground mt-2">
          {Math.round((completedCount / totalCount) * 100)}% complete
        </p>
      </motion.div>

      {/* Search */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.1 }}
        className="relative"
      >
        <Search className="absolute left-4 top-1/2 transform -translate-y-1/2 w-5 h-5 text-muted-foreground" />
        <input
          type="text"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search items..."
          className="w-full pl-12 pr-4 py-4 bg-card border border-border rounded-xl focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent transition-colors duration-200"
        />
      </motion.div>

      {/* Categories */}
      <div className="space-y-4">
        {itemsByCategory.map(category => {
          const isExpanded = expandedCategories.has(category.id);
          const Icon = category.icon;

          return (
            <motion.div
              key={category.id}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              className="card-nutrition overflow-hidden"
            >
              {/* Category Header */}
              <motion.button
                whileHover={{ scale: 1.01 }}
                onClick={() => toggleCategory(category.id)}
                className="w-full p-6 flex items-center justify-between hover:bg-muted/50 transition-colors duration-200"
              >
                <div className="flex items-center space-x-4">
                  <div className={`w-12 h-12 ${category.bgColor} rounded-xl flex items-center justify-center`}>
                    <Icon className={`w-6 h-6 ${category.textColor}`} />
                  </div>
                  <div>
                    <h3 className="text-heading-3 font-semibold">{category.name}</h3>
                    <p className="text-caption text-muted-foreground">
                      {category.items.length} items
                    </p>
                  </div>
                </div>
                <motion.div
                  animate={{ rotate: isExpanded ? 180 : 0 }}
                  transition={{ duration: 0.2 }}
                >
                  <ChevronDown className="w-5 h-5 text-muted-foreground" />
                </motion.div>
              </motion.button>

              {/* Category Items */}
              <AnimatePresence>
                {isExpanded && (
                  <motion.div
                    initial={{ height: 0, opacity: 0 }}
                    animate={{ height: 'auto', opacity: 1 }}
                    exit={{ height: 0, opacity: 0 }}
                    transition={{ duration: 0.3, ease: "easeInOut" }}
                    className="overflow-hidden"
                  >
                    <div className="px-6 pb-6">
                      {category.items.length > 0 ? (
                        <Reorder.Group
                          axis="y"
                          values={category.items}
                          onReorder={(newItems) => {
                            const updatedItems = items.map(item =>
                              item.category === category.id
                                ? newItems.find(newItem => newItem.id === item.id) || item
                                : item
                            );
                            setItems(updatedItems);
                          }}
                          className="space-y-3"
                        >
                          {category.items.map(item => (
                            <ShoppingListItem
                              key={item.id}
                              item={item}
                              onToggle={toggleItem}
                              onDelete={deleteItem}
                              onEdit={editItem}
                            />
                          ))}
                        </Reorder.Group>
                      ) : (
                        <motion.div
                          initial={{ opacity: 0, scale: 0.95 }}
                          animate={{ opacity: 1, scale: 1 }}
                          className="text-center py-8"
                        >
                          <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mx-auto mb-4">
                            <Icon className="w-8 h-8 text-muted-foreground" />
                          </div>
                          <p className="text-body text-muted-foreground">
                            No items in this category yet
                          </p>
                        </motion.div>
                      )}
                    </div>
                  </motion.div>
                )}
              </AnimatePresence>
            </motion.div>
          );
        })}
      </div>

      {/* Add Item Modal */}
      <ShoppingListModal
        isOpen={showAddModal}
        onClose={() => setShowAddModal(false)}
        onItemAdded={addItem}
      />
    </div>
  );
}