import * as React from "react";
import { Checkbox as CheckboxPrimitive } from "@base-ui/react/checkbox";
import { Check } from "lucide-react";
import { cn } from "../../lib/utils";

export interface CheckboxProps extends React.ComponentPropsWithoutRef<
  typeof CheckboxPrimitive.Root
> {}

const Checkbox = React.forwardRef<
  React.ElementRef<typeof CheckboxPrimitive.Root>,
  CheckboxProps
>(({ className, ...props }, ref) => (
  <CheckboxPrimitive.Root
    ref={ref}
    className={cn(
      "peer flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900/90 shadow-2xs transition-all duration-150",
      "hover:border-zinc-400 dark:hover:border-zinc-500 hover:bg-zinc-50 dark:hover:bg-zinc-800/60",
      "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 focus-visible:ring-offset-1 dark:focus-visible:ring-offset-zinc-950",
      "data-checked:bg-blue-600 dark:data-checked:bg-blue-500 data-checked:border-blue-600 dark:data-checked:border-blue-500 data-checked:text-white dark:data-checked:text-white",
      "disabled:cursor-not-allowed disabled:opacity-50 select-none cursor-pointer",
      className,
    )}
    {...props}
  >
    <CheckboxPrimitive.Indicator className="flex items-center justify-center text-current data-unchecked:hidden">
      <Check className="h-3 w-3 stroke-[2.75] transition-transform duration-150 animate-in zoom-in-75" />
    </CheckboxPrimitive.Indicator>
  </CheckboxPrimitive.Root>
));
Checkbox.displayName = "Checkbox";

export { Checkbox };
