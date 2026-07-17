// The top rail: product plate, a build/mock indicator, and the primary action.

interface Props {
  onAdd: () => void
  mock: boolean
}

export function TopBar({ onAdd, mock }: Props) {
  return (
    <header className="flex shrink-0 items-center justify-between border-b border-rule bg-rack px-4 py-2">
      <div className="flex items-baseline gap-3">
        <span className="font-plate text-base font-semibold uppercase tracking-[0.2em] text-readout">
          Oikos
        </span>
        <span className="plate">fleet control</span>
        {mock && (
          <span className="plate rounded-sm bg-signal/10 px-1.5 py-0.5 text-signal">demo data</span>
        )}
      </div>
      <div className="flex items-center gap-3">
        <span className="plate flex items-center gap-1 border border-rule px-2 py-1 text-plate">⌘K palette</span>
        <button
          onClick={onAdd}
          className="border border-signal bg-signal/10 px-3 py-1.5 font-plate text-xs uppercase tracking-wider text-signal transition-colors duration-led hover:bg-signal/20"
        >
          + Add peer
        </button>
      </div>
    </header>
  )
}
