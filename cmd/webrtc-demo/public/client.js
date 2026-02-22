/* WebSocket compatibility wrapper using JSON messages */
function createSocket() {
    let ws;
    const handlers = new Map();
    let connected = false;
    let id = null;

    let reconnectAttempts = 0;
    let reconnectTimer = null;
    let intentionallyClosed = false;
    let isReconnecting = false; // 防止並發重新連線

    function scheduleReconnect() {
        if (intentionallyClosed) return;
        if (isReconnecting) {
            console.log('WebSocket: 重新連線已在進行中，跳過');
            return;
        }
        
        reconnectAttempts = Math.min(reconnectAttempts + 1, 6); // cap exponent
        const delay = Math.min(30000, Math.pow(2, reconnectAttempts - 1) * 1000); // 1s,2s,4s,...,30s
        console.log(`WebSocket: scheduling reconnect in ${delay}ms (嘗試 ${reconnectAttempts}/6)`);
        
        if (reconnectTimer) clearTimeout(reconnectTimer);
        reconnectTimer = setTimeout(() => {
            connect();
        }, delay);
    }

    function connect() {
        if (isReconnecting) {
            console.log('WebSocket: 連線已在進行中，跳過');
            return;
        }
        
        intentionallyClosed = false;
        isReconnecting = true;
        
        // Connect to WebSocket through the proxy (使用當前頁面的 protocol 和 domain)
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const url = `${protocol}//${window.location.host}/ws`;

        try {
            ws = new WebSocket(url);
        } catch (e) {
            console.error('WebSocket connect error', e);
            isReconnecting = false;
            scheduleReconnect();
            return;
        }

        ws.addEventListener('open', () => {
            // connected will be confirmed when server sends 'connected' message with id
            console.log('WebSocket open');
            isReconnecting = false;
        });

        ws.addEventListener('message', (ev) => {
            try {
                const msg = JSON.parse(ev.data);
                const type = msg.type;
                const payload = msg.payload;

                if (type === 'connected') {
                    id = payload.id;
                    connected = true;
                    reconnectAttempts = 0;
                    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
                    if (handlers.has('connect')) handlers.get('connect')();
                    return;
                }

                if (handlers.has(type)) {
                    handlers.get(type)(payload);
                }
            } catch (e) {
                console.error('invalid message', e);
            }
        });

        ws.addEventListener('close', (ev) => {
            connected = false;
            id = null;
            isReconnecting = false;
            if (handlers.has('disconnect')) handlers.get('disconnect')();
            // schedule reconnect with backoff
            console.log('WebSocket closed', ev);
            scheduleReconnect();
        });

        ws.addEventListener('error', (e) => {
            connected = false;
            isReconnecting = false;
            if (handlers.has('error')) handlers.get('error')(e);
            console.log('WebSocket error', e);
            // schedule reconnect
            scheduleReconnect();
        });
    }

    // start connection
    connect();

    return {
        on: (event, cb) => handlers.set(event, cb),
        emit: (event, payload) => {
            try {
                if (ws && ws.readyState === WebSocket.OPEN) {
                    ws.send(JSON.stringify({ type: event, payload: payload || null }));
                } else {
                    console.warn('WebSocket not open, cannot send', event);
                }
            } catch (e) {
                console.error('send error', e);
            }
        },
        connect: () => { if (!connected) connect(); },
        close: () => { intentionallyClosed = true; if (reconnectTimer) clearTimeout(reconnectTimer); if (ws) ws.close(); },
        get connected() { return connected; },
        get id() { return id; }
    };
}

const socket = createSocket();

let localStream = null;
let processedStream = null;
let peerConnections = new Map();
let myUserId = null;
let myUserName = null;
let audioContext = null;
let localAnalyser = null;
let localGainNode = null;
let remoteAnalysers = new Map();
let audioGainNodes = new Map();

// ICE 伺服器位置
const iceServers = {
    iceServers: [
        { urls: 'stun:stun.l.google.com:19302' },
        { urls: 'stun:stun1.l.google.com:19302' }
    ]
};

// DOM 元素
const joinBtn = document.getElementById('joinBtn');
const joinModal = document.getElementById('joinModal');
const userName = document.getElementById('userName');
const confirmJoin = document.getElementById('confirmJoin');
const cancelJoin = document.getElementById('cancelJoin');
const participantList = document.getElementById('participantList');
const statusDiv = document.getElementById('status');
const audioStreams = document.getElementById('audioStreams');

// 事件監聽器
joinBtn.addEventListener('click', handleJoinLeaveBtn);
confirmJoin.addEventListener('click', joinChat);
cancelJoin.addEventListener('click', hideJoinModal);

socket.on('connect', () => {
    myUserId = socket.id;
    updateStatus('伺服器連線正常');
    console.log('✅ 連線到伺服器，ID:', myUserId);
    
    // 自動取得目前線上使用者列表
    socket.emit('get-online-users');
});

socket.on('connect_error', (error) => {
    console.error('❌ Socket 連線錯誤:', error);
    updateStatus('伺服器連線失敗');
});

socket.on('connect_timeout', () => {
    console.error('❌ Socket 連線超時');
    updateStatus('連線超時');
});

socket.on('error', (error) => {
    console.error('❌ Socket 錯誤:', error);
    if (error.message) {
        alert(`伺服器錯誤: ${error.message}`);
    }
});

// 收到線上使用者列表（僅用於顯示，不建立連線）
socket.on('online-users-list', (users) => {
    console.log('收到線上使用者列表:', users);
    
    // 清空參與者列表
    participantList.innerHTML = '';
    
    if (users.length === 0) {
        // 目前沒有線上使用者，顯示空狀態
        showEmptyState();
    } else {
        // 顯示所有線上使用者（只顯示，不建立 WebRTC 連線）
        users.forEach(user => {
            addParticipant(user.userId, user.userName, false, true);
        });
    }
});

socket.on('disconnect', () => {
    updateStatus('伺服器連線中斷');
    console.log('與伺服器斷開連線');
    
    // 斷線時清理狀態
    if (myUserName) {
        // 如果已經加入，保持當前狀態，等待重新連線
        console.log('等待重新連線...');
    }
    
    // 自動嘗試重新連線
    attemptReconnect();
});

// 重新連線
let reconnectIntervalId = null; // 防止創建多個 interval
function attemptReconnect() {
    if (socket.connected) {
        console.log('已經連線，無需重新連線');
        return;
    }
    
    // 如果已經有重新連線的 interval 在運行，不要再創建
    if (reconnectIntervalId) {
        console.log('重新連線已在進行中，跳過');
        return;
    }
    
    console.log('嘗試重新連線...');
    updateStatus('嘗試連線中...');
    
    // 每 3 秒嘗試一次重新連線
    reconnectIntervalId = setInterval(() => {
        if (socket.connected) {
            console.log('重新連線成功');
            clearInterval(reconnectIntervalId);
            reconnectIntervalId = null;
            
            // 如果之前已經加入聊天室，重新加入
            if (myUserName) {
                console.log('重新加入聊天室:', myUserName);
                socket.emit('join-room', { userName: myUserName });
            }
        } else {
            console.log('重新連線中...');
            socket.connect();
        }
    }, 3000); 
    
    // 30 秒後停止自動重連
    setTimeout(() => {
        if (!socket.connected && reconnectIntervalId) {
            clearInterval(reconnectIntervalId);
            reconnectIntervalId = null;
            updateStatus('無法連線到伺服器，請重新整理頁面');
            console.log('重新連線失敗，已停止嘗試');
        }
    }, 30000);
}

// 收到目前線上使用者列表
socket.on('current-users', async (users) => {
    console.log('收到當前使用者列表:', users);
    
    // 確保先清理可能存在的舊資料（自己除外）
    const myParticipant = document.getElementById(`participant-${myUserId}`);
    participantList.querySelectorAll('.participant').forEach(p => {
        if (p.id !== `participant-${myUserId}`) {
            p.remove();
        }
    });
    
    // 加入所有其他使用者
    for (const user of users) {
        addParticipant(user.userId, user.userName);
        // 向每個現有使用者建立連線
        await createPeerConnection(user.userId, true);
    }
});

// 有新使用者加入
socket.on('user-joined', async (data) => {
    console.log('新使用者加入:', data);
    
    // 將新使用者加入參與者列表
    // 如果自己還沒加入聊天室，以僅查看模式顯示
    const isViewOnly = !myUserName;
    addParticipant(data.userId, data.userName, false, isViewOnly);
    
    // 如果自己已經加入聊天室，需要等待對方發送 offer
    if (myUserName) {
        console.log('等待新使用者發送連線請求...');
    }
});

// 使用者離開
socket.on('user-left', (userId) => {
    console.log('使用者離開:', userId);
    removeParticipant(userId);
    closePeerConnection(userId);
});

// 收到 WebRTC offer
socket.on('offer', async (data) => {
    console.log('收到 offer 來自:', data.sender);
    await handleOffer(data.sender, data.offer);
});

// 收到 WebRTC answer
socket.on('answer', async (data) => {
    console.log('收到 answer 來自:', data.sender);
    await handleAnswer(data.sender, data.answer);
});

// 收到 ICE candidate
socket.on('ice-candidate', async (data) => {
    console.log('收到 ICE candidate 來自:', data.sender);
    await handleIceCandidate(data.sender, data.candidate);
});

// UI 函數
function handleJoinLeaveBtn() {
    if (joinBtn.dataset.state === 'joined') {
        leaveChat();
    } else {
        showJoinModal();
    }
}

function showJoinModal() {
    // 嘗試從本機儲存區讀取名稱
    const savedUserName = localStorage.getItem('webrtc-username');
    if (savedUserName && !userName.value.trim()) {
        userName.value = savedUserName;
        console.log('自動填入本機儲存的名稱:', savedUserName);
    }
    
    joinModal.classList.add('show');
    // 如果已經有使用者名稱，聚焦到確認按鈕；否則聚焦到輸入框
    if (userName.value.trim()) {
        setTimeout(() => confirmJoin.focus(), 100);
    } else {
        userName.focus();
    }
}

function hideJoinModal() {
    joinModal.classList.remove('show');
}

function updateStatus(message, isConnected = false) {
    statusDiv.textContent = message;
    if (isConnected) {
        statusDiv.classList.add('connected');
    } else {
        statusDiv.classList.remove('connected');
    }
}

// 驗證使用者名稱（僅允許英文、數字、CJK 文字）
function validateUserName(name) {
    if (!name || typeof name !== 'string') {
        return { valid: false, error: '名稱不可為空' };
    }
    
    const trimmedName = name.trim();
    
    if (trimmedName.length === 0) {
        return { valid: false, error: '名稱不可為空' };
    }
    
    if (trimmedName.length > 20) {
        return { valid: false, error: '名稱長度不可超過 20 個字元' };
    }
    
    // 只允許英文字母、數字、CJK 文字（中日韓統一表意文字）
    const validPattern = /^[a-zA-Z0-9\u4e00-\u9fff\u3400-\u4dbf\u20000-\u2a6df\u2a700-\u2b73f\u2b740-\u2b81f\u2b820-\u2ceaf\uac00-\ud7af\u3040-\u309f\u30a0-\u30ff]+$/;
    
    if (!validPattern.test(trimmedName)) {
        return { valid: false, error: '名稱只能包含英文、數字、中文、日文、韓文' };
    }
    
    return { valid: true, name: trimmedName };
}

async function joinChat() {
    const name = userName.value.trim();
    
    // 驗證名稱格式
    const validation = validateUserName(name);
    if (!validation.valid) {
        alert(validation.error);
        return;
    }
    
    const validatedName = validation.name;

    try {
        // 請求麥克風權限
        localStream = await navigator.mediaDevices.getUserMedia({ 
            audio: true, 
            video: false 
        });
        
        // 初始化音訊系統（使用者交互後，AudioContext 可以恢復）
        initAudioContext();
        
        // 強制恢復 AudioContext（Android 需要）
        if (audioContext && audioContext.state === 'suspended') {
            await audioContext.resume();
            console.log('AudioContext 恢復成功，狀態:', audioContext.state);
        }
        
        myUserName = validatedName;
        hideJoinModal();
        
        // 將使用者名稱儲存到 localStorage（重新整理後可自動重新加入）
        localStorage.setItem('webrtc-username', validatedName);
        
        // 清空參與者列表（包括空狀態提示）
        participantList.innerHTML = '';
        
        // 添加自己到參與者列表（必須先創建 DOM 元素）
        addParticipant(myUserId, validatedName, true);
        
        // 關鍵：在加入房間前先設置音訊處理（在 DOM 元素創建之後）
        setupLocalAudioAnalyser();
        
        // 等待處理後的音訊流就緒
        await new Promise(resolve => setTimeout(resolve, 100));
        
        console.log('processedStream 已就緒:', !!processedStream);
        
        // 加入房間（觸發伺服器發送當前使用者列表）
        socket.emit('join-room', { userName: validatedName });
        
        // 更改按鈕為離開狀態
        joinBtn.innerHTML = '<i class="fas fa-times"></i><span>離開通話</span>';
        joinBtn.dataset.state = 'joined';
        joinBtn.style.background = 'linear-gradient(135deg, #ff5722 0%, #f44336 100%)';
        
        updateStatus('已加入聊天室', true);
        console.log('成功加入聊天室');
    } catch (error) {
        console.error('無法取得麥克風權限:', error);
        let errorMsg = '需要麥克風權限才能加入聊天室。\n\n';
        
        if (error.name === 'NotAllowedError') {
            errorMsg += '❌ 權限被拒絕\n請在瀏覽器設定中允許使用麥克風。';
        } else if (error.name === 'NotFoundError') {
            errorMsg += '❌ 找不到麥克風設備\n請確認您的設備有麥克風。';
        } else if (error.name === 'NotSupportedError') {
            errorMsg += '❌ 不支援的操作\n可能原因：\n1. 需要使用 HTTPS 連線\n2. 瀏覽器不支援此功能';
        } else if (error.name === 'NotReadableError') {
            errorMsg += '❌ 無法讀取麥克風\n麥克風可能被其他應用程式占用。';
        } else if (error.name === 'SecurityError') {
            errorMsg += '❌ 安全性錯誤\n請透過 HTTPS 連線存取此網站。';
        } else {
            errorMsg += `錯誤: ${error.message}`;
        }
        
        alert(errorMsg);
        hideJoinModal();
    }
}

function addParticipant(userId, name, isMe = false, isViewOnly = false) {
    // 如果已經存在，不重複加入
    if (document.getElementById(`participant-${userId}`)) {
        console.log(`參與者 ${name} 已存在，跳過添加`);
        return;
    }

    console.log(`加入參與者: ${name} (${userId})${isMe ? ' [我]' : ''}${isViewOnly ? ' [僅查看]' : ''}`);

    // 如果列表是空的，移除空狀態提示
    const emptyState = participantList.querySelector('.empty-state');
    if (emptyState) {
        console.log('移除空狀態提示');
        emptyState.remove();
    }

    const participant = document.createElement('div');
    participant.className = `participant ${isMe ? 'me' : ''} ${isViewOnly ? 'view-only' : ''}`;
    participant.id = `participant-${userId}`;
    
    const initial = name.charAt(0).toUpperCase();
    
    // 建立參與者左側區塊
    const leftDiv = document.createElement('div');
    leftDiv.className = 'participant-left';
    
    const iconDiv = document.createElement('div');
    iconDiv.className = 'participant-icon';
    iconDiv.textContent = initial;
    
    const infoDiv = document.createElement('div');
    infoDiv.className = 'participant-info';
    
    const nameDiv = document.createElement('div');
    nameDiv.className = 'participant-name';
    nameDiv.textContent = name;
    
    const statusDiv = document.createElement('div');
    statusDiv.className = 'participant-status';
    statusDiv.textContent = isViewOnly ? '通話中' : (isMe ? '(你)' : '線上');
    
    infoDiv.appendChild(nameDiv);
    infoDiv.appendChild(statusDiv);
    leftDiv.appendChild(iconDiv);
    leftDiv.appendChild(infoDiv);
    participant.appendChild(leftDiv);
    
    // 如果不是僅查看模式，加入音量控制
    if (!isViewOnly) {
        const centerDiv = document.createElement('div');
        centerDiv.className = 'participant-center';
        
        const volumeDisplay = document.createElement('div');
        volumeDisplay.className = 'volume-display';
        
        const volumeBar = document.createElement('div');
        volumeBar.className = 'volume-bar';
        
        const volumeLevel = document.createElement('div');
        volumeLevel.className = 'volume-level';
        volumeLevel.id = `volume-${userId}`;
        
        volumeBar.appendChild(volumeLevel);
        volumeDisplay.appendChild(volumeBar);
        centerDiv.appendChild(volumeDisplay);
        participant.appendChild(centerDiv);
        
        const rightDiv = document.createElement('div');
        rightDiv.className = 'participant-right';
        
        const icon = document.createElement('i');
        icon.className = `fas ${isMe ? 'fa-microphone' : 'fa-volume-up'}`;
        icon.style.marginRight = '8px';
        
        const slider = document.createElement('input');
        slider.type = 'range';
        slider.className = 'volume-slider';
        slider.id = `slider-${userId}`;
        slider.min = '0';
        slider.max = '200';
        slider.value = '100';
        
        rightDiv.appendChild(icon);
        rightDiv.appendChild(slider);
        participant.appendChild(rightDiv);
    }
    
    participantList.appendChild(participant);

    // 只有非僅查看模式才綁定音量控制事件監聽器
    if (!isViewOnly) {
        // 使用 setTimeout 確保 DOM 完全渲染後再綁定事件監聽器
        // 避免在初始化時意外觸發 input 事件
        setTimeout(() => {
            const slider = document.getElementById(`slider-${userId}`);
            if (slider) {
                slider.addEventListener('input', (e) => {
                    const volume = e.target.value / 100;
                    if (isMe) {
                        // 調整自己的麥克風音量
                        adjustLocalVolume(volume);
                    } else {
                        // 調整對方的音量
                        adjustRemoteVolume(userId, volume);
                    }
                });
            }
        }, 0);
    }
}

function leaveChat() {
    if (!confirm('確定要離開聊天室嗎？')) {
        return;
    }
    
    console.log('離開聊天室');
    
    // 通知伺服器使用者離開（重要：這樣伺服器會通知其他使用者）
    socket.emit('leave-room');
    
    // 清除保存的使用者名
    localStorage.removeItem('webrtc-username');
    
    // 停止本機音訊流
    if (localStream) {
        localStream.getTracks().forEach(track => {
            track.stop();
            console.log('停止音軌:', track.kind);
        });
        localStream = null;
    }
    
    // 清理處理後的流
    if (processedStream) {
        processedStream.getTracks().forEach(track => track.stop());
        processedStream = null;
    }
    
    // 清理本機增益節點
    localGainNode = null;
    
    // 關閉所有 peer 連線
    peerConnections.forEach((pc, userId) => {
        console.log('關閉連線:', userId);
        pc.close();
    });
    peerConnections.clear();
    
    // 清理音訊分析器和增益節點
    remoteAnalysers.clear();
    audioGainNodes.clear();
    localAnalyser = null;
    
    // 關閉音訊系統
    if (audioContext) {
        audioContext.close();
        audioContext = null;
    }
    
    // 清空音訊元素
    audioStreams.innerHTML = '';
    
    // 只移除自己，保留其他在線使用者
    const myParticipant = document.getElementById(`participant-${myUserId}`);
    if (myParticipant) {
        myParticipant.remove();
    }
    
    // 重新取得線上使用者列表，以僅查看模式顯示
    socket.emit('get-online-users');
    
    // 重置按鈕狀態
    joinBtn.innerHTML = '<i class="fas fa-plus"></i><span>加入通話</span>';
    joinBtn.dataset.state = 'notJoined';
    joinBtn.style.background = ''; // 移除 inline style，使用 CSS 默認樣式
    
    // 更新狀態
    updateStatus('已連線到伺服器');
    
    // 重置使用者名
    myUserName = null;
    
    console.log('已成功離開聊天室');
}

function removeParticipant(userId) {
    const participant = document.getElementById(`participant-${userId}`);
    if (participant) {
        participant.remove();
    }

    // 如果沒有任何參與者，顯示空狀態
    if (participantList.children.length === 0) {
        showEmptyState();
    }
}

function showEmptyState() {
    const emptyState = document.createElement('div');
    emptyState.className = 'empty-state';
    
    const p = document.createElement('p');
    p.textContent = '目前沒有其他人在聊天室';
    
    const small = document.createElement('small');
    small.textContent = '點擊右上角的 + 加入聊天';
    
    emptyState.appendChild(p);
    emptyState.appendChild(small);
    participantList.appendChild(emptyState);
}

// WebRTC 函數
async function createPeerConnection(userId, createOffer = false) {
    if (peerConnections.has(userId)) {
        console.log('連線已存在:', userId);
        return;
    }

    console.log('建立 peer connection 給:', userId);
    const peerConnection = new RTCPeerConnection(iceServers);
    peerConnections.set(userId, peerConnection);

    // 加入處理後的音訊流（含增益控制）
    const streamToSend = processedStream || localStream;
    
    if (!streamToSend) {
        console.error('❌ 沒有可用的音訊流！processedStream 和 localStream 都不存在');
        console.error('這會導致 WebRTC 連線失敗！');
        return;
    }
    
    console.log('準備發送音訊流:', {
        'processedStream 存在': !!processedStream,
        'localStream 存在': !!localStream,
        '實際使用': streamToSend === processedStream ? '處理後的流' : '原始流',
        '音軌數量': streamToSend.getTracks().length,
        '音軌狀態': streamToSend.getTracks().map(t => ({ kind: t.kind, enabled: t.enabled, readyState: t.readyState }))
    });
    
    if (streamToSend) {
        streamToSend.getTracks().forEach(track => {
            peerConnection.addTrack(track, streamToSend);
            console.log('✅ 加入音軌到 peer connection:', track.kind, track.label, '狀態:', track.readyState);
        });
    }

    // 處理遠端音訊流
    peerConnection.ontrack = (event) => {
        console.log('收到遠端音訊流:', userId);
        console.log('  - Streams:', event.streams.length);
        console.log('  - Track kind:', event.track.kind);
        console.log('  - Track enabled:', event.track.enabled);
        console.log('  - Track readyState:', event.track.readyState);
        
        if (event.streams && event.streams[0]) {
            const stream = event.streams[0];
            console.log('  - Stream tracks:', stream.getTracks().length);
            stream.getTracks().forEach(track => {
                console.log('    * Track:', track.kind, track.enabled, track.readyState);
            });
            handleRemoteStream(userId, stream);
        } else {
            console.error('沒有收到有效的音訊流');
        }
    };

    // 處理 ICE candidates
    peerConnection.onicecandidate = (event) => {
        if (event.candidate) {
            console.log('發送 ICE candidate 到:', userId);
            socket.emit('ice-candidate', {
                target: userId,
                candidate: event.candidate
            });
        }
    };

    // 監聽連線狀態
    peerConnection.onconnectionstatechange = () => {
        console.log(`連線狀態 (${userId}):`, peerConnection.connectionState);
        
        if (peerConnection.connectionState === 'failed') {
            console.error('❌ WebRTC 連線失敗！可能原因：');
            console.error('1. 設備在不同的網路環境（需要 TURN 伺服器）');
            console.error('2. 防火牆或 NAT 阻擋了 UDP 連接');
            console.error('3. 網路不穩定');
            
            // 嘗試重新建立連線（僅一次）
            if (!peerConnection.retryAttempted) {
                peerConnection.retryAttempted = true;
                console.log('⚠️  嘗試重新建立連線...');
                
                setTimeout(async () => {
                    // 關閉舊連線
                    closePeerConnection(userId);
                    
                    // 重新建立連線
                    await createPeerConnection(userId, true);
                    console.log('✅ 已發起重連請求');
                }, 1000);
            } else {
                console.error('❌ 重連失敗，建議：');
                console.error('- 確保兩個設備在同一區域網路');
                console.error('- 或者設定 TURN 伺服器');
                
                // 顯示給使用者
                setTimeout(() => {
                    alert('無法建立與對方的連線。\n\n可能原因：\n1. 設備不在同一網路環境\n2. 防火牆或路由器阻擋\n\n建議：\n- 確保兩個設備連接到相同的 Wi-Fi\n- 或聯絡管理員設定 TURN 伺服器');
                }, 500);
                
                closePeerConnection(userId);
            }
        } else if (peerConnection.connectionState === 'disconnected') {
            console.warn('⚠️  連線中斷，等待恢復...');
            // disconnected 狀態可能是暫時的，等待 30 秒
            setTimeout(() => {
                if (peerConnection.connectionState === 'disconnected') {
                    console.error('連線中斷超時，關閉連線');
                    closePeerConnection(userId);
                }
            }, 30000);
        } else if (peerConnection.connectionState === 'connected') {
            console.log('✅ WebRTC 連線成功！');
        }
    };

    // 監聽 ICE 連線狀態
    peerConnection.oniceconnectionstatechange = () => {
        console.log(`ICE 連線狀態 (${userId}):`, peerConnection.iceConnectionState);
        
        if (peerConnection.iceConnectionState === 'failed') {
            console.error('❌ ICE 連線失敗！建議設定 TURN 伺服器');
        } else if (peerConnection.iceConnectionState === 'connected') {
            console.log('✅ ICE 連線成功！');
        } else if (peerConnection.iceConnectionState === 'checking') {
            console.log('🔍 正在嘗試建立 ICE 連線...');
        }
    };

    // 如果需要建立 offer
    if (createOffer) {
        try {
            const offer = await peerConnection.createOffer();
            await peerConnection.setLocalDescription(offer);
            console.log('發送 offer 到:', userId);
            socket.emit('offer', {
                target: userId,
                offer: offer
            });
        } catch (error) {
            console.error('建立 offer 失敗:', error);
        }
    }
}

async function handleOffer(senderId, offer) {
    // 如果連線不存在，先建立一個
    if (!peerConnections.has(senderId)) {
        await createPeerConnection(senderId, false);
    }

    const peerConnection = peerConnections.get(senderId);
    
    try {
        await peerConnection.setRemoteDescription(new RTCSessionDescription(offer));
        const answer = await peerConnection.createAnswer();
        await peerConnection.setLocalDescription(answer);
        
        console.log('發送 answer 到:', senderId);
        socket.emit('answer', {
            target: senderId,
            answer: answer
        });
    } catch (error) {
        console.error('處理 offer 失敗:', error);
    }
}

async function handleAnswer(senderId, answer) {
    const peerConnection = peerConnections.get(senderId);
    if (peerConnection) {
        try {
            await peerConnection.setRemoteDescription(new RTCSessionDescription(answer));
            console.log('設定 remote description 成功');
        } catch (error) {
            console.error('處理 answer 失敗:', error);
        }
    }
}

async function handleIceCandidate(senderId, candidate) {
    const peerConnection = peerConnections.get(senderId);
    if (peerConnection) {
        try {
            await peerConnection.addIceCandidate(new RTCIceCandidate(candidate));
            console.log('加入 ICE candidate 成功');
        } catch (error) {
            console.error('加入 ICE candidate 失敗:', error);
        }
    }
}

function handleRemoteStream(userId, stream) {
    // 移除舊的音訊元素（如果存在）
    const oldAudio = document.getElementById(`audio-${userId}`);
    if (oldAudio) {
        oldAudio.remove();
    }

    // 設定遠端音訊流（含音量控制）
    setupRemoteAudioWithVolume(userId, stream);

    console.log('遠端音訊流已加入:', userId);
}

function closePeerConnection(userId) {
    const peerConnection = peerConnections.get(userId);
    if (peerConnection) {
        peerConnection.close();
        peerConnections.delete(userId);
    }

    // 移除音訊元素
    const audio = document.getElementById(`audio-${userId}`);
    if (audio) {
        audio.remove();
    }

    // 清理分析器與增益節點
    remoteAnalysers.delete(userId);
    audioGainNodes.delete(userId);

    console.log('關閉 peer connection:', userId);
}

// 頁面載入時顯示初始狀態
window.addEventListener('load', () => {
    console.log('=== 頁面已載入 ===');
    console.log('WebSocket wrapper 已載入, socket id:', socket.id);
    console.log('Socket 連線狀態:', socket.connected);
    console.log('Socket ID:', socket.id);
    
    // 檢查是否已經連線
    if (socket.connected) {
        // 已經連線，直接更新狀態
        myUserId = socket.id;
        updateStatus('已連線到伺服器');
        console.log('✅ Socket 已連線');
        // 自動取得目前線上使用者列表
        socket.emit('get-online-users');
    } else {
        // 還在連線中
        updateStatus('嘗試連線中...');
    }
    
    showEmptyState();
    
    // 如果 5 秒後仍未連線，顯示警告
    setTimeout(() => {
        if (!socket.connected) {
            console.error('❌ 5 秒後仍未連線到伺服器');
            updateStatus('無法連線到伺服器，請確認伺服器是否運作');
        }
    }, 5000);
});

// 頁面關閉時清理資源
window.addEventListener('beforeunload', () => {
    if (localStream) {
        localStream.getTracks().forEach(track => track.stop());
    }
    peerConnections.forEach((pc) => pc.close());
    if (audioContext) {
        audioContext.close();
    }
});

// 監聽瀏覽器可見性變化（處理瀏覽器靜止後恢復的情況）
document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
        console.log('📱 瀏覽器恢復顯示');
        
        // 檢查連線狀態
        if (!socket.connected) {
            console.log('⚠️  檢測到斷線，嘗試重新連線');
            
            // 清理可能存在的舊 interval
            if (reconnectIntervalId) {
                clearInterval(reconnectIntervalId);
                reconnectIntervalId = null;
            }
            
            // 延遲一下再重新連線，避免瀏覽器剛恢復時的不穩定狀態
            setTimeout(() => {
                if (!socket.connected) {
                    socket.connect();
                }
            }, 500);
        } else {
            console.log('✅ 連線正常');
            
            // 如果已加入聊天室，驗證狀態
            if (myUserName) {
                socket.emit('get-online-users');
            }
        }
    } else {
        console.log('📱 瀏覽器進入背景');
    }
});

// 音量控制相關函數
function initAudioContext() {
    if (!audioContext) {
        audioContext = new (window.AudioContext || window.webkitAudioContext)();
        console.log('建立 AudioContext，初始狀態:', audioContext.state);
    }
    
    // 確保 AudioContext 處於運作狀態（Android 需要使用者交互後恢復）
    if (audioContext.state === 'suspended') {
        console.log('AudioContext 處於 suspended 狀態，嘗試恢復中...');
        audioContext.resume().then(() => {
            console.log('AudioContext 已恢復，目前狀態:', audioContext.state);
        }).catch(err => {
            console.error('恢復 AudioContext 失敗:', err);
        });
    }
}

function setupLocalAudioAnalyser() {
    if (!audioContext || !localStream) {
        console.log('無法設置本機音訊分析器:', { audioContext: !!audioContext, localStream: !!localStream });
        if (localStream) {
            processedStream = localStream;
        }
        return;
    }

    try {
        const source = audioContext.createMediaStreamSource(localStream);
        localAnalyser = audioContext.createAnalyser();
        localAnalyser.fftSize = 256;
        localAnalyser.smoothingTimeConstant = 0.8;
        
        // 創建增益節點用於本機音量控制（真正調整麥克風音量）
        localGainNode = audioContext.createGain();
        localGainNode.gain.value = 1.0; // 默認 100%
        
        // 創建 destination 來輸出處理後的音訊流
        const destination = audioContext.createMediaStreamDestination();
        
        // 連線: 源 -> 增益節點 -> 分析器 -> destination
        source.connect(localGainNode);
        localGainNode.connect(localAnalyser);
        localAnalyser.connect(destination);
        
        // 保存處理後的音訊流（這個流會發送給其他使用者）
        processedStream = destination.stream;
        
        // 保存增益節點引用用於音量調整
        audioGainNodes.set(myUserId, localGainNode);

        console.log('本機音訊分析器設置完成，使用者ID:', myUserId);
        console.log('麥克風增益節點已創建，初始音量: 100%');
        
        // 開始監測本機音量
        monitorVolume(localAnalyser, myUserId);
    } catch (error) {
        console.error('設置本機音訊分析器失敗:', error);
        // 降級方案：直接使用原始流
        processedStream = localStream;
        console.warn('⚠️  音訊處理失敗，使用原始音訊流');
    }
}

function setupRemoteAudioWithVolume(userId, stream) {
    console.log('設置遠端音訊，使用者ID:', userId, 'stream tracks:', stream.getTracks().length);
    
    // 確保 AudioContext 處於運作狀態（如果存在的話）
    if (audioContext && audioContext.state === 'suspended') {
        console.log('AudioContext suspended，嘗試恢復...');
        audioContext.resume().then(() => {
            console.log('AudioContext 已恢復');
        }).catch(err => {
            console.warn('AudioContext 恢復失敗:', err);
        });
    }

    try {
        // 創建 audio 元素來實際播放音訊（Android/iOS 必須）
        const audio = document.createElement('audio');
        audio.id = `audio-${userId}`;
        audio.srcObject = stream;
        audio.autoplay = true;
        audio.playsInline = true; // iOS 需要
        audio.muted = false; // 確保不靜音
        audio.volume = 1.0; // 初始音量
        
        // iOS 特殊處理：設置音訊屬性
        if (/iPad|iPhone|iPod/.test(navigator.userAgent)) {
            audio.setAttribute('webkit-playsinline', 'true');
            audio.setAttribute('playsinline', 'true');
            console.log('🍎 iOS 設備：已設置 playsinline 屬性');
        }
        
        // 添加錯誤處理
        audio.onerror = (e) => {
            console.error('Audio 元素錯誤:', e);
            console.error('錯誤類型:', audio.error ? audio.error.code : 'unknown');
        };
        
        // 監聽播放狀態
        audio.onloadedmetadata = () => {
            console.log('遠端音訊中繼資料已載入:', userId);
            console.log('音訊流狀態:', {
                tracks: stream.getTracks().length,
                active: stream.active,
                audioTrack: stream.getAudioTracks()[0]?.enabled
            });
            
            // 嘗試播放
            const playPromise = audio.play();
            if (playPromise !== undefined) {
                playPromise.then(() => {
                    console.log('✅ 遠端音訊開始播放:', userId);
                }).catch(err => {
                    console.error('❌ 播放遠端音訊失敗:', err);
                    console.log('等待使用者互動以開始播放...');
                    
                    // 使用者互動後重試播放
                    const retryPlay = () => {
                        audio.play().then(() => {
                            console.log('✅ 使用者互動後音訊開始播放');
                        }).catch(e => {
                            console.error('重試播放仍失敗:', e);
                        });
                    };
                    
                    // 監聽多種用戶互動事件
                    document.addEventListener('click', retryPlay, { once: true });
                    document.addEventListener('touchstart', retryPlay, { once: true });
                });
            }
        };
        
        // 監聽播放事件
        audio.onplay = () => {
            console.log('🔊 音訊播放事件觸發:', userId);
        };
        
        audio.onpause = () => {
            console.log('⏸️  音訊暫停事件:', userId);
        };
        
        audioStreams.appendChild(audio);
        
        // 創建音訊分析器（用於音量顯示）- 這是可選的
        if (audioContext) {
            try {
                const source = audioContext.createMediaStreamSource(stream);
                const analyser = audioContext.createAnalyser();
                analyser.fftSize = 256;
                analyser.smoothingTimeConstant = 0.8;
                remoteAnalysers.set(userId, analyser);
                
                // 只連線到分析器，不連線到 destination（避免重複播放）
                source.connect(analyser);
                
                // 開始監測音量
                monitorVolume(analyser, userId);
            } catch (analyserError) {
                console.warn('無法創建音訊分析器（不影響播放）:', analyserError);
            }
        } else {
            console.warn('⚠️  AudioContext 不存在，跳過音量視覺化');
        }
        
        // 保存 audio 元素引用到 audioGainNodes（用於音量控制）
        audioGainNodes.set(userId, audio);
        
        console.log('遠端音訊設定完成，使用者 ID:', userId);
    } catch (error) {
        console.error('設定遠端音訊失敗:', error);
        console.error('錯誤詳情:', error.message, error.stack);
    }
}

function monitorVolume(analyser, userId) {
    const bufferLength = analyser.frequencyBinCount;
    const dataArray = new Uint8Array(bufferLength);
    
    function updateVolume() {
        // 檢查元素是否還存在
        const volumeBar = document.getElementById(`volume-${userId}`);
        if (!volumeBar) {
            return;
        }
        
        // 使用頻域數據獲取音量（更準確）
        analyser.getByteFrequencyData(dataArray);
        
        // 計算平均音量
        let sum = 0;
        for (let i = 0; i < bufferLength; i++) {
            sum += dataArray[i];
        }
        const average = sum / bufferLength;
        const volumePercent = Math.min(100, (average / 255) * 150); // 調整顯示效果
        
        // 更新音量條
        volumeBar.style.width = `${volumePercent}%`;
        requestAnimationFrame(updateVolume);
    }
    
    updateVolume();
}

function adjustLocalVolume(volume) {
    // 調整麥克風輸入音量（會影響發送給其他人的音量）
    if (localGainNode) {
        const oldValue = localGainNode.gain.value;
        localGainNode.gain.value = volume;
        console.log(`調整麥克風音量: ${(oldValue * 100).toFixed(0)}% -> ${(volume * 100).toFixed(0)}%`);
        console.log('localGainNode.gain.value =', localGainNode.gain.value);
    } else {
        console.error('本機增益節點不存在，無法調整音量');
        console.error('請確認 setupLocalAudioAnalyser() 已被調用');
    }
}

function adjustRemoteVolume(userId, volume) {
    // 直接調整 audio 元素的音量
    const audio = audioGainNodes.get(userId); // 這裡存的是 audio 元素
    if (audio && audio.volume !== undefined) {
        try {
            audio.volume = Math.max(0, Math.min(2, volume)); // 確保在 0-2 範圍內
            console.log(`調整遠端音量 [${userId}]: ${(volume * 100).toFixed(0)}%`);
        } catch (error) {
            console.error(`調整遠端音量失敗 [${userId}]:`, error);
        }
    } else {
        // 遠端 audio 元素還未創建，這是正常的（連線尚未建立）
        console.log(`遠端 audio 元素尚未就緒 [${userId}]，跳過音量調整`);
    }
}
