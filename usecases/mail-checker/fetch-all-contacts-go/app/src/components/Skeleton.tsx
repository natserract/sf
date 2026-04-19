type Props = { height?: number; style?: React.CSSProperties };

export default function Skeleton({ height = 80, style }: Props) {
  return <div className="skeleton" style={{ height, borderRadius: 8, ...style }} />;
}
