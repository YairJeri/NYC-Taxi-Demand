import { h } from 'preact';

export default function Sparkline({ data = [], color = '#6366f1', height = 40, width = 100, strokeWidth = 2 }) {
    if (!data || data.length === 0) return <div style={{ height: `${height}px` }} class="w-full flex items-center justify-center text-xs text-slate-700">No data</div>;

    const max = Math.max(...data) || 100;
    const min = Math.min(0, Math.min(...data));
    const range = max - min || 1;

    const points = data.map((val, i) => {
        const x = (i / (data.length - 1 || 1)) * width;
        const y = height - ((val - min) / range) * height;
        return `${x},${y}`;
    }).join(' ');

    return (
        <svg
            viewBox={`0 -${strokeWidth} ${width} ${height + strokeWidth * 2}`}
            class="w-full overflow-visible"
            style={{ height: `${height}px` }}
            preserveAspectRatio="none"
        >
            <polyline
                fill="none"
                stroke={color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
                strokeLinejoin="round"
                points={points}
            />
            {/* Optional gradient fill under the line */}
            <polygon
                fill={`url(#gradient-${color.replace('#', '')})`}
                points={`${points} ${width},${height} 0,${height}`}
                opacity="0.2"
            />
            <defs>
                <linearGradient id={`gradient-${color.replace('#', '')}`} x1="0" x2="0" y1="0" y2="1">
                    <stop offset="0%" stop-color={color} stop-opacity="1" />
                    <stop offset="100%" stop-color={color} stop-opacity="0" />
                </linearGradient>
            </defs>
        </svg>
    );
}
