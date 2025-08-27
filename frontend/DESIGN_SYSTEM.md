# Nutreek Design System & UI/UX Documentation

## 🎨 Overview

Nutreek is a modern, vibrant nutrition planning web application designed with accessibility and user experience as top priorities. This design system provides comprehensive guidelines for maintaining consistency across all components and features.

## 🌈 Color Palette

### Primary Color Scheme
Our color palette is inspired by fresh, healthy foods and uses vibrant, energetic colors while maintaining accessibility standards.

```css
/* Primary Colors */
--primary: 142 76% 36%;        /* Vibrant Green */
--secondary: 45 93% 47%;       /* Bright Yellow */
--accent: 199 89% 48%;         /* Ocean Blue */
--destructive: 0 84% 60%;     /* Accessible Red */

/* Neutral Colors */
--background: 120 20% 98%;     /* Off-white */
--foreground: 120 10% 15%;     /* Dark Charcoal */
--muted: 120 10% 95%;          /* Light Gray */
--border: 120 20% 90%;         /* Border Gray */

/* Nutrition-Specific Colors */
--nutrition-protein: 25 95% 53%;   /* Orange */
--nutrition-carbs: 45 93% 47%;     /* Yellow */
--nutrition-fat: 280 100% 70%;     /* Pink */
--nutrition-fiber: 120 60% 50%;    /* Green */
--nutrition-calories: 0 70% 50%;   /* Red */

/* Meal Type Colors */
--meal-breakfast: 45 93% 47%;       /* Yellow */
--meal-snack: 142 76% 36%;         /* Green */
--meal-lunch: 199 89% 48%;         /* Blue */
--meal-dinner: 280 100% 70%;       /* Pink */
```

### Dark Mode Support
The design system includes comprehensive dark mode support with carefully adjusted colors for optimal contrast and readability.

## 📝 Typography

### Font Hierarchy

```css
/* Display Text */
.text-display {
  font-size: 3xl/4xl/5xl/6xl;
  font-weight: 700;
  line-height: tight;
  letter-spacing: -0.025em;
}

/* Headings */
.text-heading-1 { font-size: 3xl/4xl; font-weight: 700; }
.text-heading-2 { font-size: 2xl/3xl; font-weight: 600; }
.text-heading-3 { font-size: xl/2xl; font-weight: 600; }

/* Body Text */
.text-body-large { font-size: lg; line-height: relaxed; }
.text-body { font-size: base; line-height: relaxed; }
.text-caption { font-size: sm; color: muted-foreground; }
```

### Font Family
- **Primary**: Inter (Google Fonts) - Clean, modern sans-serif
- **Fallback**: System font stack for optimal performance

## 🎭 Animations & Micro-interactions

### Animation Guidelines

#### Timing & Easing
- **Quick Interactions**: 150-200ms (buttons, hover states)
- **Page Transitions**: 300-400ms (route changes, modal open/close)
- **Complex Animations**: 500-600ms (initial page loads, major state changes)
- **Easing**: Use spring animations for natural feel (framer-motion default)

#### Animation Triggers
- **Hover**: Scale (1.02-1.05x), slight lift, color transitions
- **Tap/Click**: Scale down (0.95-0.98x) with quick return
- **Focus**: Subtle glow effect with smooth transitions
- **Loading**: Skeleton animations or spinner with breathing effect
- **Success/Error**: Bounce animation with color feedback

### Reduced Motion Support
The design system respects `prefers-reduced-motion` media query:
```css
@media (prefers-reduced-motion: reduce) {
  .animate-fade-in,
  .animate-slide-up,
  .animate-scale-in {
    animation: none;
  }
}
```

## 🧩 Component Library

### 1. Dashboard Layout
- **Responsive sidebar** with collapsible navigation
- **Skip links** for keyboard accessibility
- **Search bar** with animated focus states
- **Notification center** with animated badges

### 2. Weekly Planner
- **Animated meal cards** with vibrant color coding
- **Drag-and-drop** recipe assignment
- **Progress indicators** for meal completion
- **Empty states** with engaging illustrations

### 3. Ingredient Search
- **Typeahead functionality** with smooth animations
- **Nutrition-focused results** with color-coded macros
- **Keyboard navigation** support
- **Loading states** with skeleton animations

### 4. Substitution Manager
- **Multi-step modal** with progress indicator
- **Nutrition comparison** visualization
- **Smooth transitions** between steps
- **Confirmation dialogs** with animations

### 5. Shopping List
- **Collapsible categories** with smooth animations
- **Drag-and-drop** reordering
- **Animated checkboxes** with completion feedback
- **Progress tracking** with visual indicators

## ♿ Accessibility (WCAG AA Compliance)

### Color Contrast
- **Normal Text**: 4.5:1 minimum contrast ratio
- **Large Text**: 3:1 minimum contrast ratio
- **Interactive Elements**: 3:1 minimum contrast ratio
- **Focus Indicators**: Clear visual focus with 2px minimum width

### Keyboard Navigation
- **Tab Order**: Logical and intuitive navigation flow
- **Focus Indicators**: Visible focus rings on all interactive elements
- **Skip Links**: Quick navigation to main content and navigation
- **Keyboard Shortcuts**: Custom shortcuts for power users

### Screen Reader Support
- **Semantic HTML**: Proper use of headings, landmarks, and roles
- **ARIA Labels**: Comprehensive labeling for complex components
- **Live Regions**: Dynamic content announcements
- **Hidden Labels**: Screen reader only text for clarity

### Focus Management
- **Focus Trapping**: Modal dialogs trap focus appropriately
- **Initial Focus**: Proper initial focus placement
- **Focus Return**: Focus returns to trigger element after modal close
- **Focus Indicators**: High contrast focus indicators

## 📱 Responsive Design

### Breakpoints
- **Mobile**: < 640px
- **Tablet**: 640px - 1024px
- **Desktop**: > 1024px

### Responsive Patterns
- **Mobile-first approach** with progressive enhancement
- **Flexible grid systems** that adapt to screen size
- **Touch-friendly** button sizes (minimum 44px)
- **Optimized typography** scaling across devices

## 🎯 Interaction Patterns

### Buttons
```css
.btn-nutrition {
  @apply bg-primary text-primary-foreground hover:bg-primary/90
         transition-colors duration-200 font-medium rounded-lg px-4 py-2;
}

.btn-nutrition-secondary {
  @apply bg-secondary text-secondary-foreground hover:bg-secondary/90
         transition-colors duration-200 font-medium rounded-lg px-4 py-2;
}
```

### Cards
```css
.card-nutrition {
  @apply bg-card border border-border rounded-xl shadow-sm
         hover:shadow-md transition-all duration-200;
}
```

### Form Elements
- **Focus states** with smooth color transitions
- **Validation feedback** with animated error messages
- **Loading states** with skeleton animations
- **Success states** with checkmark animations

## 🎨 Visual Design Principles

### 1. Vibrant & Energetic
- Use of bright, nutrition-inspired colors
- Energetic gradients and color combinations
- Bold typography and high contrast elements

### 2. Soft & Modern
- **Border Radius**: 8px-16px for modern feel
- **Shadows**: Soft shadows for depth without harshness
- **Spacing**: Generous white space for breathing room
- **Typography**: Clean, readable fonts with proper hierarchy

### 3. Accessible by Design
- **High Contrast**: WCAG AA compliant color combinations
- **Clear Focus**: Obvious focus indicators for keyboard users
- **Readable Text**: Appropriate font sizes and line heights
- **Touch Targets**: Adequate size for touch interfaces

### 4. Consistent & Scalable
- **Design Tokens**: Centralized color, spacing, and typography
- **Component Library**: Reusable, consistent components
- **Pattern Library**: Established interaction patterns
- **Documentation**: Comprehensive guidelines for maintenance

## 🚀 Performance Considerations

### Animation Performance
- Use `transform` and `opacity` for GPU acceleration
- Avoid animating layout properties when possible
- Implement `will-change` for complex animations
- Respect `prefers-reduced-motion` setting

### Bundle Optimization
- Lazy load components for better initial load times
- Optimize images and icons
- Use efficient CSS classes and avoid inline styles
- Implement code splitting for large components

## 🛠️ Development Guidelines

### Component Structure
```typescript
interface ComponentProps {
  // Required props
  // Optional props with defaults
  // Event handlers
  className?: string; // For style customization
}

// Use forwardRef for focus management
const Component = forwardRef<HTMLDivElement, ComponentProps>(
  ({ className, ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={cn("base-classes", className)}
        {...props}
      >
        {/* Component content */}
      </div>
    );
  }
);
```

### Animation Best Practices
- **Purposeful Animations**: Every animation should serve a purpose
- **Performance First**: Optimize for 60fps on all devices
- **User Respect**: Always check for reduced motion preferences
- **Consistent Timing**: Use established timing curves throughout

### Accessibility Checklist
- [ ] Semantic HTML elements
- [ ] Proper ARIA attributes
- [ ] Keyboard navigation support
- [ ] Screen reader compatibility
- [ ] Color contrast compliance
- [ ] Focus management
- [ ] Touch target sizes
- [ ] Alternative text for images

## 📊 Metrics & Success Criteria

### Performance Goals
- **Lighthouse Score**: > 90 for Performance, Accessibility, SEO
- **Core Web Vitals**: All metrics in "Good" range
- **Bundle Size**: < 200KB for initial load
- **Animation Performance**: 60fps on target devices

### Accessibility Goals
- **WCAG AA Compliance**: 100% compliance across all components
- **Keyboard Navigation**: Full functionality without mouse
- **Screen Reader Support**: Comprehensive support for assistive technologies
- **Color Contrast**: All text meets or exceeds WCAG AA standards

### User Experience Goals
- **Task Completion Rate**: > 95% for core user flows
- **Error Rate**: < 5% for form submissions
- **User Satisfaction**: > 4.5/5 in user testing
- **Loading Performance**: < 2 seconds for initial page load

## 🔄 Maintenance & Updates

### Design Token Updates
When updating design tokens:
1. Update CSS custom properties in `globals.css`
2. Update Tailwind config if needed
3. Test across all components
4. Update documentation
5. Run accessibility audits

### Component Updates
When modifying components:
1. Maintain backward compatibility
2. Update TypeScript interfaces
3. Test accessibility features
4. Update component documentation
5. Run cross-browser testing

### Animation Updates
When adding new animations:
1. Follow established timing guidelines
2. Implement reduced motion support
3. Test performance impact
4. Document animation purpose and usage

---

This design system ensures that Nutreek provides a modern, accessible, and delightful user experience while maintaining consistency and performance across all features and devices.