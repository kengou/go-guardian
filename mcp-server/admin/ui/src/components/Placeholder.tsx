interface PlaceholderProps {
  name: string;
}

export function Placeholder({ name }: PlaceholderProps) {
  return (
    <div class="view-container placeholder">
      <h2>{name}</h2>
      <p class="placeholder-text">Coming soon</p>
    </div>
  );
}
