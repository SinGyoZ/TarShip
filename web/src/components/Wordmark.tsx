export function Wordmark() {
  return (
    <span className="inline-flex items-center gap-2 text-lg font-semibold tracking-tight">
      <svg viewBox="0 0 32 32" className="size-6 text-accent" aria-hidden="true">
        <rect width="32" height="32" rx="7" fill="currentColor" />
        <path d="M7 19h18l-3 5H10zM15 6h2v11h-2zM17 7l7 9h-7z" fill="white" />
      </svg>
      TarShip
    </span>
  );
}
