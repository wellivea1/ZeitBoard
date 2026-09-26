// What a view says while its first read is in flight. One wording everywhere,
// and never an empty or unavailable state: nothing has been found missing yet.
export function Loading({ children = "Reading your records…" }: { children?: string }) {
  return (
    <p className="screen-loading" role="status">
      {children}
    </p>
  );
}
