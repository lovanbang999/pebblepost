import * as React from "react";
import { RadioGroup as RadioGroupPrimitive } from "@base-ui/react/radio-group";
import { Radio as RadioPrimitive } from "@base-ui/react/radio";
import { Circle } from "lucide-react";
import { cn } from "../../lib/utils";

export interface RadioGroupProps
  extends React.ComponentPropsWithoutRef<typeof RadioGroupPrimitive> {}

const RadioGroup = React.forwardRef<HTMLDivElement, RadioGroupProps>(
  ({ className, ...props }, ref) => {
    return (
      <RadioGroupPrimitive
        ref={ref}
        className={cn("flex items-center gap-4", className)}
        {...props}
      />
    );
  },
);
RadioGroup.displayName = "RadioGroup";

export interface RadioGroupItemProps
  extends React.ComponentPropsWithoutRef<typeof RadioPrimitive.Root> {}

const RadioGroupItem = React.forwardRef<HTMLSpanElement, RadioGroupItemProps>(
  ({ className, ...props }, ref) => {
    return (
      <RadioPrimitive.Root
        ref={ref}
        className={cn(
          "peer aspect-square h-4 w-4 rounded-full border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 shadow-2xs transition-all duration-150",
          "hover:border-zinc-400 dark:hover:border-zinc-500 hover:bg-zinc-50 dark:hover:bg-zinc-800/60",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 focus-visible:ring-offset-1 dark:focus-visible:ring-offset-zinc-950",
          "data-checked:border-blue-600 dark:data-checked:border-blue-500 data-checked:bg-white dark:data-checked:bg-zinc-950",
          "disabled:cursor-not-allowed disabled:opacity-50 select-none cursor-pointer flex items-center justify-center",
          className,
        )}
        {...props}
      >
        <RadioPrimitive.Indicator className="flex items-center justify-center">
          <Circle className="h-2 w-2 fill-blue-600 dark:fill-blue-500 text-blue-600 dark:text-blue-500 stroke-0 transition-transform duration-150 animate-in zoom-in-50" />
        </RadioPrimitive.Indicator>
      </RadioPrimitive.Root>
    );
  },
);
RadioGroupItem.displayName = "RadioGroupItem";

export { RadioGroup, RadioGroupItem };
