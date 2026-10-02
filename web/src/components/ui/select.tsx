import * as React from "react";
import { Select as SelectPrimitive } from "@base-ui/react/select";
import { Check, ChevronDown } from "lucide-react";
import { cn } from "../../lib/utils";

export const Select = SelectPrimitive.Root;
export const SelectGroup = SelectPrimitive.Group;
export const SelectValue = SelectPrimitive.Value;

export interface SelectTriggerProps extends React.ComponentPropsWithoutRef<
  typeof SelectPrimitive.Trigger
> {
  hideChevron?: boolean;
}

export const SelectTrigger = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Trigger>,
  SelectTriggerProps
>(({ className, children, hideChevron = false, ...props }, ref) => (
  <SelectPrimitive.Trigger
    ref={ref}
    className={cn(
      "flex h-8 w-full items-center justify-between gap-1.5 rounded-md border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-2.5 py-1 text-xs text-zinc-900 dark:text-zinc-100 shadow-xs transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/60 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50 select-none cursor-pointer",
      className,
    )}
    {...props}
  >
    {children}
    {!hideChevron && (
      <SelectPrimitive.Icon className="shrink-0 opacity-50">
        <ChevronDown className="h-3 w-3" />
      </SelectPrimitive.Icon>
    )}
  </SelectPrimitive.Trigger>
));
SelectTrigger.displayName = "SelectTrigger";

export interface SelectContentProps extends React.ComponentPropsWithoutRef<
  typeof SelectPrimitive.Popup
> {
  sideOffset?: number;
  align?: "center" | "start" | "end";
  side?: "top" | "bottom" | "left" | "right";
  alignItemWithTrigger?: boolean;
}

export const SelectContent = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Popup>,
  SelectContentProps
>(
  (
    {
      className,
      children,
      sideOffset = 4,
      align = "start",
      side = "bottom",
      alignItemWithTrigger = false,
      ...props
    },
    ref,
  ) => (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Positioner
        side={side}
        sideOffset={sideOffset}
        align={align}
        alignItemWithTrigger={alignItemWithTrigger}
        className="z-50 select-none outline-none"
      >
        <SelectPrimitive.Popup
          ref={ref}
          className={cn(
            "relative z-50 min-w-32 overflow-hidden rounded-md border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-lg shadow-black/10 dark:shadow-black/30",
            "origin-(--transform-origin) transition-[transform,opacity] duration-100 ease-out",
            "data-starting-style:scale-95 data-starting-style:opacity-0",
            "data-ending-style:scale-95 data-ending-style:opacity-0",
            "data-instant:transition-none",
            className,
          )}
          {...props}
        >
          <SelectPrimitive.List className="p-1 max-h-72 overflow-y-auto">
            {children}
          </SelectPrimitive.List>
        </SelectPrimitive.Popup>
      </SelectPrimitive.Positioner>
    </SelectPrimitive.Portal>
  ),
);
SelectContent.displayName = "SelectContent";

export const SelectLabel = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.GroupLabel>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.GroupLabel>
>(({ className, ...props }, ref) => (
  <SelectPrimitive.GroupLabel
    ref={ref}
    className={cn(
      "px-2 py-1.5 text-xs font-semibold text-zinc-500 dark:text-zinc-400",
      className,
    )}
    {...props}
  />
));
SelectLabel.displayName = "SelectLabel";

export const SelectItem = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Item>
>(({ className, children, ...props }, ref) => (
  <SelectPrimitive.Item
    ref={ref}
    className={cn(
      "relative flex w-full cursor-pointer select-none items-center rounded-sm py-1.5 pl-6 pr-2 text-xs outline-none",
      "data-highlighted:bg-zinc-100 dark:data-highlighted:bg-zinc-800/80 data-highlighted:text-zinc-900 dark:data-highlighted:text-zinc-100",
      "data-disabled:pointer-events-none data-disabled:opacity-50 transition-colors",
      className,
    )}
    {...props}
  >
    <span className="absolute left-1.5 flex h-3.5 w-3.5 items-center justify-center">
      <SelectPrimitive.ItemIndicator>
        <Check className="h-3 w-3 text-blue-500" />
      </SelectPrimitive.ItemIndicator>
    </span>
    <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
  </SelectPrimitive.Item>
));
SelectItem.displayName = "SelectItem";

export const SelectSeparator = React.forwardRef<
  React.ElementRef<typeof SelectPrimitive.Separator>,
  React.ComponentPropsWithoutRef<typeof SelectPrimitive.Separator>
>(({ className, ...props }, ref) => (
  <SelectPrimitive.Separator
    ref={ref}
    className={cn("-mx-1 my-1 h-px bg-zinc-200 dark:bg-zinc-800", className)}
    {...props}
  />
));
SelectSeparator.displayName = "SelectSeparator";
