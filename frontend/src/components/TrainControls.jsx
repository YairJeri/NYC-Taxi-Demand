import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import Sparkline from './Sparkline';

export default function TrainControls() {
    const [loadingClean, setLoadingClean] = useState(false);
    const [loadingTrain, setLoadingTrain] = useState(false);
    const [logs, setLogs] = useState([]);

    const [modelType, setModelType] = useState('static');
    const [steps, setSteps] = useState(5000);
    const [status, setStatus] = useState(null);

    // --- NUEVOS ESTADOS PARA DETECTAR LA DESCONEXIÓN ---
    const [isFirstLoad, setIsFirstLoad] = useState(true);
    const [hasConnectionError, setHasConnectionError] = useState(false);

    const statusRef = useRef(status);
    useEffect(() => {
        statusRef.current = status;
    }, [status]);

    const addLog = (msg, type = 'info') => {
        setLogs(prev => [
            ...prev,
            { time: new Date().toLocaleTimeString('en-US', { hour12: false }), msg, type }
        ]);
    };

    const formatTime = (secs) => {
        if (!secs) return '00:00';
        const m = Math.floor(secs / 60).toString().padStart(2, '0');
        const s = Math.floor(secs % 60).toString().padStart(2, '0');
        return `${m}:${s}`;
    };

    // Función de polling extraída para ejecutarla inmediatamente al montar
    const fetchStatus = async () => {
        try {
            const res = await fetch('http://localhost:8080/api/status');
            if (res.ok) {
                const data = await res.json();
                const prevStatus = statusRef.current;

                if (prevStatus?.isProcessing && !data.isProcessing) {
                    addLog('Procesamiento de datos completado.', 'success');
                    setLoadingClean(false);
                }
                if (prevStatus?.isTraining && !data.isTraining) {
                    addLog(`Entrenamiento completado para el modelo ${prevStatus.activeModel}.`, 'success');
                    setLoadingTrain(false);
                }

                if (data.isProcessing) setLoadingClean(true);
                if (data.isTraining) setLoadingTrain(true);

                setStatus(data);
                setHasConnectionError(false); // Conexión exitosa
            } else {
                throw new Error(`HTTP ${res.status}`);
            }
        } catch (err) {
            if (statusRef.current !== null || isFirstLoad) {
                addLog('Desconectado del servidor. Cluster inalcanzable.', 'error');
                setStatus(null);
            }
            setHasConnectionError(true); // <--- Activamos el error visual
            setLoadingClean(false);
            setLoadingTrain(false);
        } finally {
            setIsFirstLoad(false); // Terminó el intento inicial
        }
    };

    useEffect(() => {
        fetchStatus(); // <--- Ejecutar de inmediato, no esperar 1 segundo entero
        const interval = setInterval(fetchStatus, 1000);
        return () => clearInterval(interval);
    }, []);

    const handleClean = async () => {
        setLoadingClean(true);
        addLog('Iniciando trabajo de procesamiento de datos...', 'info');
        try {
            const res = await fetch('http://localhost:8080/api/clean', { method: 'POST' });
            if (!res.ok) {
                const text = await res.text();
                throw new Error(text || res.statusText);
            }
            addLog('Procesamiento de datos iniciado exitosamente.', 'success');
        } catch (err) {
            addLog(`Clean error: ${err.message}`, 'error');
            setLoadingClean(false);
        }
    };

    const handleTrain = async (e) => {
        e.preventDefault();
        setLoadingTrain(true);
        addLog(`Validando datos e iniciando entrenamiento ${modelType} por ${steps} pasos...`, 'info');

        try {
            const res = await fetch(`http://localhost:8080/api/train?type=${modelType}&steps=${steps}`, { method: 'POST' });
            const text = await res.text();

            if (!res.ok) throw new Error(text || res.statusText);
            addLog(`Entrenamiento iniciado: ${text}`, 'success');
        } catch (err) {
            addLog(`Fallo al iniciar el entrenamiento: ${err.message}`, 'error');
            setLoadingTrain(false);
        }
    };

    // 1. ESTADO DE CARGA INICIAL
    if (isFirstLoad) {
        return <div class="flex items-center justify-center h-64 text-gray-500">Conectando al Nodo Maestro...</div>;
    }

    // 2. INTERRUPTOR GLOBAL DE ERROR (Mismo diseño que tu WorkerGrid)
    if (hasConnectionError) {
        return (
            <div class="bg-red-900 border border-red-700 text-red-200 p-4 rounded-md flex items-center gap-3 mb-6">
                <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                Cluster fuera de línea o inalcanzable (No se pudo obtener el estado de entrenamiento)
            </div>
        );
    }

    return (
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* El resto del HTML de los formularios y logs se queda exactamente igual */}
            <div class="flex flex-col gap-6">
                <div class="bg-gray-800 border border-gray-700 rounded-md p-6">
                    <h2 class="text-lg font-bold text-gray-100 mb-2">Gestión de Datos</h2>
                    <p class="text-sm text-gray-400 mb-6">Prepara y agrega el dataset desde archivos fuente hacia MongoDB antes de iniciar los trabajos de entrenamiento.</p>

                    <button onClick={handleClean} disabled={!status || loadingClean || loadingTrain} class="flex items-center gap-2 bg-gray-700 hover:bg-gray-600 text-white px-4 py-2 rounded-md font-medium disabled:opacity-50 disabled:cursor-not-allowed transition-colors">
                        {loadingClean ? (
                            <svg class="animate-spin w-4 h-4 text-white" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" stroke-opacity="0.25"></circle><path fill="currentColor" opacity="0.75" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                        ) : (
                            <svg class="w-4 h-4 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" /></svg>
                        )}
                        {!status ? 'Cluster Inaccesible' : 'Iniciar Trabajo de Procesamiento (/clean)'}
                    </button>
                </div>

                <div class="bg-gray-800 border border-gray-700 rounded-md p-6">
                    <h2 class="text-lg font-bold text-gray-100 mb-2">Configuración de Entrenamiento</h2>
                    <p class="text-sm text-gray-400 mb-6">Despacha un nuevo trabajo de entrenamiento distribuido a todos los nodos conectados.</p>

                    <form onSubmit={handleTrain} class="flex flex-col gap-4">
                        <div>
                            <label class="block text-sm font-bold text-gray-300 mb-2 uppercase">Selección de Arquitectura</label>
                            <div class="grid grid-cols-2 gap-4">
                                <label class={`flex flex-col p-4 cursor-pointer rounded-md border ${modelType === 'static' ? 'bg-blue-900 border-blue-700' : 'bg-gray-700 border-gray-600'}`}>
                                    <input type="radio" name="modelType" value="static" checked={modelType === 'static'} onChange={() => setModelType('static')} class="sr-only" />
                                    <span class={`font-bold ${modelType === 'static' ? 'text-blue-100' : 'text-gray-200'}`}>Modelo A (Estático)</span>
                                    <span class="text-sm text-gray-400 mt-1">Demanda Espacial (6 características)</span>
                                </label>

                                <label class={`flex flex-col p-4 cursor-pointer rounded-md border ${modelType === 'temporal' ? 'bg-blue-900 border-blue-700' : 'bg-gray-700 border-gray-600'}`}>
                                    <input type="radio" name="modelType" value="temporal" checked={modelType === 'temporal'} onChange={() => setModelType('temporal')} class="sr-only" />
                                    <span class={`font-bold ${modelType === 'temporal' ? 'text-blue-100' : 'text-gray-200'}`}>Modelo B (Temporal)</span>
                                    <span class="text-sm text-gray-400 mt-1">Series de Tiempo (31 características)</span>
                                </label>
                            </div>
                        </div>

                        <div>
                            <label class="block text-sm font-bold text-gray-300 mb-2 uppercase">Pasos Objetivo de Entrenamiento</label>
                            <input type="number" value={steps} onChange={(e) => setSteps(parseInt(e.target.value))} min="1" max="100000" class="w-full bg-gray-700 border border-gray-600 text-gray-100 rounded-md px-3 py-2" />
                        </div>

                        <div class="mt-2">
                            <button type="submit" disabled={!status || loadingTrain || loadingClean} class="w-full flex items-center justify-center gap-2 bg-blue-600 hover:bg-blue-500 text-white px-4 py-2.5 rounded-md font-bold disabled:opacity-50 disabled:cursor-not-allowed transition-colors">
                                {loadingTrain ? (
                                    <svg class="animate-spin w-4 h-4 text-white" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" stroke-opacity="0.25"></circle><path fill="currentColor" opacity="0.75" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                                ) : (
                                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" /></svg>
                                )}
                                {!status ? 'Cluster Inaccesible' : 'Despachar Trabajo de Entrenamiento'}
                            </button>
                        </div>
                    </form>
                </div>
            </div>

            <div class="bg-gray-800 border border-gray-700 rounded-md flex flex-col h-[500px]">
                <div class="p-4 border-b border-gray-700 flex justify-between items-center bg-gray-750">
                    <h2 class="font-bold text-gray-100 flex items-center gap-2">
                        <svg class="w-4 h-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7" /></svg>
                        Registros de Ejecución del Trabajo
                    </h2>
                    {logs.length > 0 && (
                        <button onClick={() => setLogs([])} class="text-sm text-gray-400 hover:text-white font-medium">Limpiar</button>
                    )}
                </div>

                <div class="flex-1 overflow-y-auto p-4 font-mono text-sm bg-gray-900 flex flex-col gap-2">
                    {logs.length === 0 ? (
                        <div class="text-gray-500 h-full flex items-center justify-center">Esperando eventos...</div>
                    ) : (
                        logs.map((log, i) => (
                            <div key={i} class="border-b border-gray-800 pb-2 break-all">
                                <span class="text-gray-500 mr-2">[{log.time}]</span>
                                <span class={
                                    log.type === 'error' ? 'text-red-400' :
                                        log.type === 'success' ? 'text-green-400' :
                                            'text-gray-300'
                                }>
                                    {log.msg}
                                </span>
                            </div>
                        ))
                    )}
                </div>

                {(status?.isProcessing || status?.isTraining) && (
                    <div class="p-4 bg-gray-800 border-t border-gray-700">
                        {status.isProcessing && (
                            <div class="mb-4">
                                <div class="flex justify-between text-sm font-bold mb-2">
                                    <span class="text-green-500 flex items-center gap-2">
                                        <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" stroke-opacity="0.25"></circle><path fill="currentColor" opacity="0.75" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                                        Procesamiento de Datos Activo <span class="text-gray-400 font-mono ml-1">({formatTime(status.processingTime)})</span>
                                    </span>
                                    <span class="text-gray-400">
                                        {status.processingTotal > 0 ? `${Math.round((status.processingDone / status.processingTotal) * 100)}%` : 'Leyendo...'}
                                    </span>
                                </div>
                                <div class="w-full bg-gray-700 h-2 rounded-sm overflow-hidden">
                                    <div class="bg-green-500 h-2" style={{ width: status.processingTotal > 0 ? `${(status.processingDone / status.processingTotal) * 100}%` : '5%' }}></div>
                                </div>
                            </div>
                        )}

                        {status.isTraining && (
                            <div>
                                <div class="flex justify-between items-end mb-4">
                                    <div class="flex items-center gap-2 font-bold text-blue-400">
                                        <svg class="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" stroke-opacity="0.25"></circle><path fill="currentColor" opacity="0.75" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                                        Entrenando Modelo {status.activeModel} <span class="text-gray-400 font-mono ml-1 text-sm">({formatTime(status.trainingTime)})</span>
                                    </div>
                                    <div class="text-right flex flex-col">
                                        <span class="text-xs text-gray-500 font-bold uppercase">Pérdida Prom.</span>
                                        <span class="font-mono font-bold text-gray-200">
                                            {status.avgLoss ? status.avgLoss.toFixed(4) : '---'}
                                        </span>
                                    </div>
                                </div>

                                {status.lossHistory && status.lossHistory.length > 0 && (
                                    <div class="h-10 w-full mb-4">
                                        <Sparkline data={status.lossHistory} color="#3b82f6" height={40} />
                                    </div>
                                )}

                                <div>
                                    <div class="flex justify-between text-sm font-bold text-gray-400 mb-2">
                                        <span>Progreso</span>
                                        <span class="font-mono">{status.trainingStep} / {status.trainingTotal}</span>
                                    </div>
                                    <div class="w-full bg-gray-700 h-2 rounded-sm overflow-hidden">
                                        <div class="bg-blue-500 h-2" style={{ width: `${(status.trainingStep / status.trainingTotal) * 100}%` }}></div>
                                    </div>
                                </div>
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}