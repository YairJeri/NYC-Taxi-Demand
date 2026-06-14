import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import Sparkline from './Sparkline';

export default function WorkerGrid() {
    const [workers, setWorkers] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const historyRef = useRef({});

    const fetchMetrics = async () => {
        try {
            const res = await fetch('http://localhost:8080/api/dashboard/metrics');
            if (!res.ok) throw new Error('Failed to fetch');
            const data = await res.json();

            const currentHistory = { ...historyRef.current };

            data.forEach(w => {
                if (!currentHistory[w.worker_id]) {
                    currentHistory[w.worker_id] = { cpu: [], ram: [], loss: [] };
                }
                const h = currentHistory[w.worker_id];
                h.cpu.push(w.cpu_utilization || 0);
                h.ram.push(w.ram_usage_bytes ? w.ram_usage_bytes / 1024 / 1024 : 0);
                h.loss.push(w.current_loss || 0);

                if (h.cpu.length > 20) h.cpu.shift();
                if (h.ram.length > 20) h.ram.shift();
                if (h.loss.length > 20) h.loss.shift();
            });

            historyRef.current = currentHistory;

            const sortedWorkers = (data || []).sort((a, b) => {
                const idA = a.worker_id || '';
                const idB = b.worker_id || '';
                return idA.localeCompare(idB, undefined, { numeric: true, sensitivity: 'base' });
            });

            setWorkers(sortedWorkers);
            setError(null);
        } catch (err) {
            setError(err.message);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        fetchMetrics();
        const interval = setInterval(fetchMetrics, 1000);
        return () => clearInterval(interval);
    }, []);

    if (loading) {
        return <div class="flex items-center justify-center h-64 text-gray-500">Conectando al Nodo Maestro...</div>;
    }

    if (error) {
        return (
            <div class="bg-red-900 border border-red-700 text-red-200 p-4 rounded-md flex items-center gap-3">
                <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                Cluster fuera de línea o inalcanzable
            </div>
        );
    }

    if (workers.length === 0) {
        return (
            <div class="border border-gray-700 rounded-md p-12 text-center text-gray-500">
                <p>No hay nodos conectados al cluster.</p>
                <p class="text-sm mt-2">Esperando conexiones TCP en el puerto :9090</p>
            </div>
        );
    }

    return (
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
            {workers.map(w => {
                const isOnline = w.status === 'Online';
                const ramMB = w.ram_usage_bytes ? (w.ram_usage_bytes / 1024 / 1024).toFixed(1) : 0;
                const totalRamMB = w.total_ram_bytes ? (w.total_ram_bytes / 1024 / 1024).toFixed(0) : 0;
                const history = historyRef.current[w.worker_id] || { cpu: [], ram: [], loss: [] };

                return (
                    <div key={w.worker_id} class="bg-gray-800 border border-gray-700 rounded-md overflow-hidden">
                        <div class="p-4 border-b border-gray-700 flex justify-between items-start">
                            <div>
                                <h3 class="font-medium text-gray-200 flex items-center gap-2">
                                    <svg class="w-4 h-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" /></svg>
                                    {w.worker_id || 'Nodo Desconocido'}
                                </h3>
                                <p class="text-sm text-gray-400 mt-1">
                                    {w.cpu_cores} Núcleos • {totalRamMB}MB RAM • v{w.worker_version}
                                </p>
                            </div>
                            <span class={`px-2 py-1 text-xs uppercase font-bold rounded-sm ${isOnline
                                ? 'bg-green-700 text-white'
                                : 'bg-red-700 text-white'
                                }`}>
                                {w.status}
                            </span>
                        </div>

                        <div class="p-4 grid grid-cols-2 gap-4">
                            <div class="flex flex-col gap-1">
                                <div class="text-sm text-gray-400 flex justify-between">
                                    <span>Uso de CPU</span>
                                    <span class="text-gray-200">{w.cpu_utilization?.toFixed(1) || 0}%</span>
                                </div>
                                <Sparkline data={history.cpu} color="#3b82f6" height={30} />
                            </div>

                            <div class="flex flex-col gap-1">
                                <div class="text-sm text-gray-400 flex justify-between">
                                    <span>Uso de RAM</span>
                                    <span class="text-gray-200">{ramMB} MB</span>
                                </div>
                                <Sparkline data={history.ram} color="#8b5cf6" height={30} />
                            </div>
                        </div>

                        <div class="px-4 py-3 bg-gray-700 text-sm flex justify-between items-center">
                            <div class="flex flex-col">
                                <span class="text-xs text-gray-400">Paso</span>
                                <span class="font-mono text-gray-200">{w.current_step || 0}</span>
                            </div>
                            <div class="flex flex-col items-end">
                                <span class="text-xs text-gray-400">Pérdida Actual</span>
                                <span class="font-mono text-blue-400 font-bold">{w.current_loss ? w.current_loss.toFixed(4) : 'N/A'}</span>
                            </div>
                        </div>
                    </div>
                );
            })}
        </div>
    );
}