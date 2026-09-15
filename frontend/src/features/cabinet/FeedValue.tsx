/**
 * FeedValue is one named value of a row or of a card, in the shape the rest of the interface states
 * a detail in. Both feeds and the card of one invoice are read as lists of named values, so what one
 * of them looks like is declared once.
 */
export function FeedValue({ term, value }: { term: string; value: string }) {
  return (
    <div className="details-row">
      <dt className="details-term">{term}</dt>
      <dd className="details-value">{value}</dd>
    </div>
  );
}
