import * as React from "react";
import { Tooltip as TooltipPrimitive } from "@base-ui/react/tooltip";
import { cn } from "../../lib/utils";

export function TooltipProvider({
  delay = 200,
  ...props
}: TooltipPrimitive.Provider.Props) {
  return (
    <TooltipPrimitive.Provider
      data-slot="tooltip-provider"
      delay={delay}
      {...props}
    />
  );
}

export function TooltipTrigger({ ...props }: TooltipPrimitive.Trigger.Props) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />;
}

export interface TooltipContentProps
  extends
    TooltipPrimitive.Popup.Props,
    Pick<
      TooltipPrimitive.Positioner.Props,
      "align" | "alignOffset" | "side" | "sideOffset"
    > {
  showArrow?: boolean;
}

export function TooltipContent({
  className,
  side = "top",
  sideOffset = 8,
  align = "center",
  alignOffset = 0,
  showArrow = true,
  children,
  ...props
}: TooltipContentProps) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Positioner
        align={align}
        alignOffset={alignOffset}
        side={side}
        sideOffset={sideOffset}
        className="isolate z-50"
      >
        <TooltipPrimitive.Popup
          data-slot="tooltip-content"
          className={cn(
            "relative z-50 inline-flex w-fit max-w-xs items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium shadow-md shadow-black/15 dark:shadow-black/40 border border-foreground/10 select-none pointer-events-none",
            "bg-foreground text-background",
            "origin-(--transform-origin)",
            "transition-[transform,opacity] duration-150 ease-out",
            "data-starting-style:opacity-0 data-starting-style:scale-95",
            "data-ending-style:opacity-0 data-ending-style:scale-95",
            "data-instant:transition-none",
            className,
          )}
          {...props}
        >
          {children}
          {showArrow && (
            <TooltipPrimitive.Arrow className="relative block w-3 h-1.5 overflow-clip data-[side=bottom]:-top-1.5 data-[side=left]:-right-2.25 data-[side=left]:rotate-90 data-[side=right]:-left-2.25 data-[side=right]:-rotate-90 data-[side=top]:-bottom-1.5 data-[side=top]:rotate-180 before:content-[''] before:absolute before:bottom-0 before:left-1/2 before:w-[8.5px] before:h-[8.5px] before:bg-foreground before:border before:border-foreground/10 before:transform-[translate(-50%,50%)_rotate(45deg)]" />
          )}
        </TooltipPrimitive.Popup>
      </TooltipPrimitive.Positioner>
    </TooltipPrimitive.Portal>
  );
}

export interface TooltipProps extends TooltipPrimitive.Root.Props {
  content?: React.ReactNode;
  children?: React.ReactNode;
  side?: "top" | "bottom" | "left" | "right";
  sideOffset?: number;
  align?: "center" | "start" | "end";
  showArrow?: boolean;
  className?: string;
}

export function Tooltip({
  content,
  children,
  side = "bottom",
  sideOffset = 8,
  align = "center",
  showArrow = true,
  className,
  ...props
}: TooltipProps) {
  if (content !== undefined) {
    return (
      <TooltipPrimitive.Root {...props}>
        <TooltipPrimitive.Trigger
          render={
            React.isValidElement(children) ? children : <span>{children}</span>
          }
        />
        <TooltipContent
          side={side}
          sideOffset={sideOffset}
          align={align}
          showArrow={showArrow}
          className={className}
        >
          {content}
        </TooltipContent>
      </TooltipPrimitive.Root>
    );
  }

  return (
    <TooltipPrimitive.Root data-slot="tooltip" {...props}>
      {children}
    </TooltipPrimitive.Root>
  );
}
