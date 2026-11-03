import * as React from "react"
import { cn } from "@/lib/utils"

interface ProgressProps extends React.HTMLAttributes<HTMLDivElement> {
  value?: number
  indicatorClassName?: string
  indeterminate?: boolean
}

function Progress({ className, value = 0, indicatorClassName, indeterminate, ...props }: ProgressProps) {
  return (
    <div
      className={cn(
        "bg-primary/20 relative h-2 w-full overflow-hidden rounded-full",
        className
      )}
      role="progressbar"
      aria-valuenow={indeterminate ? undefined : value}
      aria-valuemin={0}
      aria-valuemax={100}
      {...props}
    >
      <div
        className={cn(
          "bg-primary h-full",
          indeterminate && "w-2/5 animate-progress-sweep rounded-full",
          indicatorClassName,
        )}
        style={indeterminate ? undefined : { width: `${value}%` }}
      />
    </div>
  )
}

export { Progress }
