import { useEffect, useRef, useCallback, useState } from 'react';

export function useFocusTrap(isActive: boolean) {
  const containerRef = useRef<HTMLElement>(null);

  const focusableElementsString = 'a[href], area[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), button:not([disabled]), iframe, object, embed, [tabindex="0"], [contenteditable]';

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (!isActive || !containerRef.current) return;

    const focusableElements = containerRef.current.querySelectorAll(focusableElementsString);
    const firstElement = focusableElements[0] as HTMLElement;
    const lastElement = focusableElements[focusableElements.length - 1] as HTMLElement;

    if (e.key === 'Tab') {
      if (e.shiftKey) {
        if (document.activeElement === firstElement) {
          e.preventDefault();
          lastElement?.focus();
        }
      } else {
        if (document.activeElement === lastElement) {
          e.preventDefault();
          firstElement?.focus();
        }
      }
    }

    if (e.key === 'Escape') {
      // Let parent components handle escape
      e.stopPropagation();
    }
  }, [isActive, focusableElementsString]);

  useEffect(() => {
    if (isActive) {
      document.addEventListener('keydown', handleKeyDown);
      // Focus first focusable element when trap becomes active
      const focusableElements = containerRef.current?.querySelectorAll(focusableElementsString);
      const firstElement = focusableElements?.[0] as HTMLElement;
      setTimeout(() => firstElement?.focus(), 100);
    }

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [isActive, handleKeyDown, focusableElementsString]);

  return containerRef;
}

export function useAnnounce(message: string, priority: 'polite' | 'assertive' = 'polite') {
  useEffect(() => {
    if (message) {
      // Create temporary announcement element
      const announcement = document.createElement('div');
      announcement.setAttribute('aria-live', priority);
      announcement.setAttribute('aria-atomic', 'true');
      announcement.className = 'sr-only';
      announcement.textContent = message;
      document.body.appendChild(announcement);

      // Remove after announcement
      setTimeout(() => {
        if (document.body.contains(announcement)) {
          document.body.removeChild(announcement);
        }
      }, 1000);
    }
  }, [message, priority]);
}

export function useSkipLink(targetId: string) {
  const handleSkip = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    const target = document.getElementById(targetId);
    if (target) {
      target.focus();
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }, [targetId]);

  return handleSkip;
}

export function useReducedMotion() {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(false);

  useEffect(() => {
    const mediaQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
    setPrefersReducedMotion(mediaQuery.matches);

    const handleChange = () => setPrefersReducedMotion(mediaQuery.matches);
    mediaQuery.addEventListener('change', handleChange);

    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  return prefersReducedMotion;
}

export function useHighContrast() {
  const [prefersHighContrast, setPrefersHighContrast] = useState(false);

  useEffect(() => {
    const mediaQuery = window.matchMedia('(prefers-contrast: high)');
    setPrefersHighContrast(mediaQuery.matches);

    const handleChange = () => setPrefersHighContrast(mediaQuery.matches);
    mediaQuery.addEventListener('change', handleChange);

    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  return prefersHighContrast;
}

export function useAriaDialog({ isOpen, onClose, title }: {
  isOpen: boolean;
  onClose: () => void;
  title: string;
}) {
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isOpen) {
      // Announce dialog opening
      const announcement = document.createElement('div');
      announcement.setAttribute('aria-live', 'assertive');
      announcement.setAttribute('aria-atomic', 'true');
      announcement.className = 'sr-only';
      announcement.textContent = `${title} dialog opened`;
      document.body.appendChild(announcement);

      setTimeout(() => {
        if (document.body.contains(announcement)) {
          document.body.removeChild(announcement);
        }
      }, 1000);
    }
  }, [isOpen, title]);

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose();
    }
  }, [onClose]);

  useEffect(() => {
    if (isOpen) {
      document.addEventListener('keydown', handleKeyDown);
    }
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, handleKeyDown]);

  return dialogRef;
}

// Utility for managing ARIA expanded state
export function useAriaExpanded(isExpanded: boolean, controlsId?: string) {
  return {
    'aria-expanded': isExpanded,
    'aria-controls': isExpanded ? controlsId : undefined
  };
}

// Utility for managing ARIA pressed state
export function useAriaPressed(isPressed: boolean) {
  return {
    'aria-pressed': isPressed
  };
}

// Utility for managing ARIA selected state
export function useAriaSelected(isSelected: boolean) {
  return {
    'aria-selected': isSelected
  };
}

// Utility for managing ARIA busy state
export function useAriaBusy(isBusy: boolean) {
  return {
    'aria-busy': isBusy
  };
}

// Screen reader announcement hook
export function useScreenReaderAnnounce() {
  const announce = useCallback((message: string, priority: 'polite' | 'assertive' = 'polite') => {
    const announcement = document.createElement('div');
    announcement.setAttribute('aria-live', priority);
    announcement.setAttribute('aria-atomic', 'true');
    announcement.className = 'sr-only';
    announcement.textContent = message;
    document.body.appendChild(announcement);

    setTimeout(() => {
      if (document.body.contains(announcement)) {
        document.body.removeChild(announcement);
      }
    }, 1000);
  }, []);

  return announce;
}