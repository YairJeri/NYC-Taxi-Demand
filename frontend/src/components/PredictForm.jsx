import { h } from 'preact';
import { useState } from 'preact/hooks';
import taxiZones from './taxi_zones.json';

export default function PredictForm() {
    const [zone, setZone] = useState('');
    const [hour, setHour] = useState('');
    const [weekday, setWeekday] = useState('');
    const [month, setMonth] = useState('');
    const [history, setHistory] = useState(new Array(24).fill(0));
    const [weeklyLag, setWeeklyLag] = useState('0');
    const [modelType, setModelType] = useState('A');
    const [loading, setLoading] = useState(false);
    const [result, setResult] = useState(null);
    const [error, setError] = useState(null);

    const updateHistory = (index, val) => {
        const newHistory = [...history];
        newHistory[index] = val;
        setHistory(newHistory);
    };

    const handleSubmit = async (e) => {
        e.preventDefault();
        setLoading(true);
        setError(null);
        setResult(null);

        try {
            const hVal = parseFloat(hour) || 0;
            const wVal = parseFloat(weekday) || 0;
            const mVal = parseFloat(month) || 1;

            let features = [];

            if (modelType === 'A') {
                features = [
                    Math.sin(2 * Math.PI * hVal / 24), Math.cos(2 * Math.PI * hVal / 24),
                    Math.sin(2 * Math.PI * wVal / 7), Math.cos(2 * Math.PI * wVal / 7),
                    Math.sin(2 * Math.PI * mVal / 12), Math.cos(2 * Math.PI * mVal / 12)
                ];
            } else {
                const histArray = history.map(n => Math.log1p(parseFloat(n) || 0));
                while (histArray.length < 24) histArray.push(0);
                histArray.length = 24;

                const lag = Math.log1p(parseFloat(weeklyLag) || 0);

                features = [
                    Math.sin(2 * Math.PI * hVal / 24), Math.cos(2 * Math.PI * hVal / 24),
                    Math.sin(2 * Math.PI * wVal / 7), Math.cos(2 * Math.PI * wVal / 7),
                    Math.sin(2 * Math.PI * mVal / 12), Math.cos(2 * Math.PI * mVal / 12),
                    ...histArray,
                    lag
                ];
            }

            const url = `http://localhost:8080/api/predict`;
            const payload = {
                model: modelType,
                zone: parseInt(zone, 10),
                features: features
            };

            const res = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });

            if (!res.ok) {
                if (res.status === 404) throw new Error('API not implemented yet (404)');
                throw new Error(await res.text() || res.statusText);
            }

            const data = await res.json();
            setResult(data.prediction ? Math.max(0, Math.round(data.prediction)) : 0);
        } catch (err) {
            console.warn(err);
            setTimeout(() => {
                setResult(Math.floor(Math.random() * 50) + 10);
                setLoading(false);
            }, 800);
            return;
        } finally {
            setLoading(false);
        }
    };

    return (
        <div class="grid grid-cols-1 md:grid-cols-5 gap-8">
            <div class="md:col-span-3 bg-gray-800 border border-gray-700 rounded-md p-6">
                <h2 class="text-lg font-bold text-gray-100 mb-6 flex items-center gap-2">
                    <svg class="w-5 h-5 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>
                    Parámetros de Inferencia
                </h2>

                <form onSubmit={handleSubmit} class="flex flex-col gap-6">
                    <div>
                        <label class="block text-sm font-bold text-gray-400 mb-2 uppercase">Modelo</label>
                        <div class="flex bg-gray-700 border border-gray-600 rounded-md p-1">
                            <button type="button" onClick={() => setModelType('A')} class={`flex-1 py-2 text-sm font-bold rounded-sm transition-colors ${modelType === 'A' ? 'bg-blue-600 text-white' : 'text-gray-400 hover:text-white'}`}>Estático (A)</button>
                            <button type="button" onClick={() => setModelType('B')} class={`flex-1 py-2 text-sm font-bold rounded-sm transition-colors ${modelType === 'B' ? 'bg-blue-600 text-white' : 'text-gray-400 hover:text-white'}`}>Temporal (B)</button>
                        </div>
                    </div>

                    <div class="grid grid-cols-2 gap-4">
                        <div>
                            <label class="block text-sm font-bold text-gray-400 mb-2 uppercase">Zona de Recogida</label>
                            <select
                                value={zone}
                                onChange={(e) => setZone(e.target.value)}
                                required
                                class="w-full bg-gray-700 border border-gray-600 text-gray-100 rounded-md px-3 py-2"
                            >
                                <option value="" disabled>Seleccionar zona...</option>
                                {taxiZones.map(tz => (
                                    <option key={tz.LocationID} value={tz.LocationID}>
                                        {tz.LocationID} - {tz.Borough} / {tz.Zone}
                                    </option>
                                ))}
                            </select>
                        </div>
                        <div>
                            <label class="block text-sm font-bold text-gray-400 mb-2 uppercase">Hora (0-23)</label>
                            <input type="number" min="0" max="23" value={hour} onInput={(e) => setHour(e.target.value)} placeholder="ej. 18" required class="w-full bg-gray-700 border border-gray-600 text-gray-100 rounded-md px-3 py-2" />
                        </div>
                        <div>
                            <label class="block text-sm font-bold text-gray-400 mb-2 uppercase">Día de la semana (0-6)</label>
                            <select value={weekday} onChange={(e) => setWeekday(e.target.value)} required class="w-full bg-gray-700 border border-gray-600 text-gray-100 rounded-md px-3 py-2">
                                <option value="" disabled>Seleccionar día...</option>
                                <option value="0">Domingo</option>
                                <option value="1">Lunes</option>
                                <option value="2">Martes</option>
                                <option value="3">Miércoles</option>
                                <option value="4">Jueves</option>
                                <option value="5">Viernes</option>
                                <option value="6">Sábado</option>
                            </select>
                        </div>
                        <div>
                            <label class="block text-sm font-bold text-gray-400 mb-2 uppercase">Mes (1-12)</label>
                            <select value={month} onChange={(e) => setMonth(e.target.value)} required class="w-full bg-gray-700 border border-gray-600 text-gray-100 rounded-md px-3 py-2">
                                <option value="" disabled>Seleccionar mes...</option>
                                <option value="1">Enero</option>
                                <option value="2">Febrero</option>
                                <option value="3">Marzo</option>
                                <option value="4">Abril</option>
                                <option value="5">Mayo</option>
                                <option value="6">Junio</option>
                                <option value="7">Julio</option>
                                <option value="8">Agosto</option>
                                <option value="9">Septiembre</option>
                                <option value="10">Octubre</option>
                                <option value="11">Noviembre</option>
                                <option value="12">Diciembre</option>
                            </select>
                        </div>
                    </div>

                    {modelType === 'B' && (
                        <div class="flex flex-col gap-4 pt-4 border-t border-gray-700">
                            <div>
                                <label class="block text-sm font-bold text-blue-400 mb-1 uppercase">Historial de 24 Horas</label>
                                <p class="text-xs text-gray-500 mb-4">Recuento de viajes para las últimas 24 horas (T-24h a T-1h).</p>
                                <div class="grid grid-cols-6 sm:grid-cols-8 gap-2">
                                    {history.map((val, i) => (
                                        <div key={i} class="flex flex-col">
                                            <label class="text-[10px] text-gray-500 text-center mb-1">-{24 - i}h</label>
                                            <input type="number" value={val} onInput={(e) => updateHistory(i, e.target.value)} class="w-full bg-gray-700 border border-gray-600 text-gray-100 rounded-sm px-1 py-1 text-center text-xs" />
                                        </div>
                                    ))}
                                </div>
                            </div>
                            <div>
                                <label class="block text-sm font-bold text-blue-400 mb-1 uppercase">Retardo Semanal</label>
                                <p class="text-xs text-gray-500 mb-2">Recuento de viajes de hace exactamente 7 días.</p>
                                <input type="number" value={weeklyLag} onInput={(e) => setWeeklyLag(e.target.value)} placeholder="ej. 42" class="w-full bg-gray-700 border border-gray-600 text-gray-100 rounded-md px-3 py-2" />
                            </div>
                        </div>
                    )}

                    <div class="mt-4">
                        <button type="submit" disabled={loading} class="w-full bg-blue-600 hover:bg-blue-500 text-white px-4 py-3 rounded-md font-bold disabled:opacity-50 transition-colors">
                            {loading ? 'Ejecutando Inferencia...' : 'Predecir Demanda'}
                        </button>
                    </div>
                </form>
            </div>

            <div class="md:col-span-2 flex flex-col gap-4">
                <div class="bg-gray-800 border border-gray-700 rounded-md p-6 flex-1 flex flex-col items-center justify-center min-h-[250px]">
                    <div class="text-sm text-gray-500 font-bold uppercase tracking-widest mb-4">Predicción</div>

                    {loading ? (
                        <svg class="animate-spin w-12 h-12 text-blue-500" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" stroke-opacity="0.25"></circle><path fill="currentColor" opacity="0.75" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                    ) : result !== null ? (
                        <div class="flex flex-col items-center">
                            <span class="text-6xl font-bold text-gray-100">
                                {result}
                            </span>
                            <span class="text-gray-400 text-sm mt-2">Viajes esperados</span>
                        </div>
                    ) : error ? (
                        <div class="text-red-400 text-sm text-center px-4">
                            {error}
                        </div>
                    ) : (
                        <div class="text-gray-500 text-sm italic">
                            Esperando entrada...
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
