import { IconStar } from "./icons";
import { countLabel } from "./russian-plural";

export function ratingTone(value: number) {
  if (value < 2) return "rating-tone--bad";
  if (value < 3) return "rating-tone--low";
  if (value < 4) return "rating-tone--fair";
  if (value < 4.7) return "rating-tone--good";
  return "rating-tone--great";
}

export default function Rating({ value, reviews, compact=false }: { value: number; reviews?: number; compact?: boolean }) {
  const rounded=Math.round(value);return <span className={`rating ${ratingTone(value)} ${compact?"rating--compact":""}`} aria-label={`Рейтинг ${value.toFixed(1)} из 5${reviews!==undefined?`, ${countLabel(reviews,["отзыв","отзыва","отзывов"])}`:""}`}><span className="rating__stars" aria-hidden="true">{Array.from({length:5},(_,index)=><IconStar key={index} size={compact?13:16} className={index<rounded?"is-filled":""}/>)}</span><strong>{value.toFixed(1)}</strong>{reviews!==undefined?<small>{countLabel(reviews,["отзыв","отзыва","отзывов"])}</small>:null}</span>
}
