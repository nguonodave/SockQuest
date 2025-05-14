let sock;
let currentUser = "";
let selectedRecipient = null;
let initialSelectedRecipient = null

const toggleVisibility = (elementId, show) => {
    const el = document.getElementById(elementId);
    el.classList[show ? "remove" : "add"]("hidden");
}

const getInputValue = (id) => document.getElementById(id).value

const clearAndShowChatElements = () => {
    document.getElementById("chatBox").innerHTML = "";
    ["messageInput", "sendButton", "chatBox"].forEach(id => toggleVisibility(id, true));
};

const createMessageElement = (msg) => {
    const p = document.createElement("p");
    p.textContent = msg.from === currentUser ? `You: ${msg.content}` : `${msg.from}: ${msg.content}`;
    p.style.textAlign = msg.from === currentUser ? "right" : "left";
    p.style.color = msg.from === currentUser ? "blue" : "green";
    return p;
};

const showLoginForm = () => {
    toggleVisibility("loginForm", true)
    toggleVisibility("registerForm", false)
};

const showRegisterForm = () => {
    toggleVisibility("registerForm", true)
    toggleVisibility("loginForm", false)
};

document.getElementById("showLogin").onclick = showLoginForm;
document.getElementById("showRegister").onclick = showRegisterForm;

document.getElementById("registerButton").onclick = async () => {
    const username = getInputValue("regUsername")
    const password = getInputValue("regPassword")

    const res = await fetch("/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password })
    });

    const data = await res.json();
    alert(data.message);
};

document.getElementById("loginButton").onclick = async () => {
    const username = getInputValue("loginUsername")
    const password = getInputValue("loginPassword")

    const res = await fetch("/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password })
    });

    const data = await res.json();

    if (data.success) {
        currentUser = username;
        localStorage.setItem("chat_user", username); // store session
        startChat();
    } else {
        alert(data.message);
    }
};

function startChat() {
    toggleVisibility("authSection", false)
    toggleVisibility("chatSection", true)
    document.getElementById("currentUser").textContent = currentUser;

    // Initially hide the message input and chat box
    ["messageInput", "sendButton", "chatBox"].forEach(id => toggleVisibility(id, false));

    connectWebSocket();
    loadUsers();
    loadUnreadCounts();
}

let unreadCounts = {};

function connectWebSocket() {
    sock = new WebSocket("ws://localhost:8080/ws");

    sock.onopen = () => {
        sock.send(JSON.stringify({
            from: currentUser,
            to: "",
            content: "",
            timestamp: new Date().toISOString()
        }));
        console.log("WebSocket connected.");
    };

    sock.onmessage = (event) => {
        const msg = JSON.parse(event.data);

        if (msg.type === "userlist") {
            populateUserList(msg.data);
        } else if (msg.to === currentUser) {
            if (msg.from === selectedRecipient) {
                const chatBox = document.getElementById("chatBox")
                const scrollbarAtBottom = chatBox.scrollHeight - chatBox.scrollTop === chatBox.clientHeight
                chatBox.appendChild(createMessageElement(msg));
                if (scrollbarAtBottom) {
                    chatBox.scrollTop = chatBox.scrollHeight
                }
                // // Mark as read when viewing the conversation
                // markMessagesAsRead(msg.from);
            } else {
                // Increment unread count for this sender
                unreadCounts[msg.from] = (unreadCounts[msg.from] || 0) + 1;
                updateNotificationBadge();
            }
        }
    };
}

function updateNotificationBadge() {
    const totalUnread = Object.values(unreadCounts).reduce((sum, count) => sum + count, 0);
    const badge = document.getElementById("notificationBadge");
    const countElement = document.getElementById("notificationCount");

    if (totalUnread > 0) {
        countElement.textContent = totalUnread;
        toggleVisibility("notificationBadge", true);
    } else {
        toggleVisibility("notificationBadge", false);
    }
}

async function loadUnreadCounts() {
    try {
        const response = await fetch(`/unreadCounts?username=${currentUser}`);
        if (response.ok) {
            unreadCounts = await response.json();
            updateNotificationBadge();
        }
    } catch (error) {
        console.error("Failed to load unread counts:", error);
    }
}

async function markMessagesAsRead(fromUser) {
    // if fromUser is not selected or there are no unread counts from fromUser, return
    if (!fromUser || unreadCounts[fromUser] === undefined) return;

    try {
        const response = await fetch("/markAsRead", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ currentUser, fromUser })
        });

        if (response.ok) {
            delete unreadCounts[fromUser];
            updateNotificationBadge();
        }
    } catch (error) {
        console.error("Failed to mark messages as read:", error);
    }
}

function populateUserList(users) {
    if (!users) {
        return
    }

    const userList = document.getElementById("userList");
    userList.innerHTML = "";

    users.forEach(user => {
        if (user.username === currentUser) return;

        const li = document.createElement("li");
        li.textContent = `${user.username} - ${user.status}`;
        li.onclick = () => openConversationWith(user.username);
        userList.appendChild(li);
    });
}

async function openConversationWith(username) {
    selectedRecipient = username;

    if (selectedRecipient === initialSelectedRecipient) {
        return
    }

    initialSelectedRecipient = selectedRecipient
    clearAndShowChatElements();

    attachScrollListener()

    const convRes = await fetch(`/conversation?currentUser=${currentUser}&selectedUser=${selectedRecipient}`);
    const messages = await convRes.json();
    const chatBox = document.getElementById("chatBox");
    if (messages) {
        messages.forEach(msg => chatBox.appendChild(createMessageElement(msg)));
    }
    chatBox.scrollTop = chatBox.scrollHeight;
    markMessagesAsRead(username)
}

let offset = 10
const limit = 10
let allMessagesLoaded = false
let isLoading = false

async function loadMessages() {
    if (allMessagesLoaded || isLoading) return

    isLoading = true
    try {
        const res = await fetch(`/conversation?currentUser=${currentUser}&selectedUser=${selectedRecipient}&limit=${limit}&offset=${offset}`)
        const messages = await res.json()

        if (!messages) {
            return
        }

        // if fewer than requested limit are returned, means all messages have been requested
        if (messages.length < limit) {
            allMessagesLoaded = true
        }

        const chatBox = document.getElementById("chatBox")
        const scrollHeightBefore = chatBox.scrollHeight

        messages.reverse().forEach(msg => {
            const el = createMessageElement(msg)
            chatBox.insertBefore(el, chatBox.firstChild)
        })

        // preserve scroll position after prepending
        chatBox.scrollTop += chatBox.scrollHeight - scrollHeightBefore
        offset += messages.length
    } finally {
        isLoading = false
    }
}

function throttle(fn, delay) {
    let lastCall = 0;
    return (...args) => {
        const now = new Date().getTime();
        if (now - lastCall >= delay) {
            lastCall = now;
            fn(...args);
        }
    };
}

function attachScrollListener() {
    const chatBox = document.getElementById("chatBox")
    chatBox.addEventListener("scroll", throttle(() => {
        console.log(chatBox.scrollTop)
        if (chatBox.scrollTop < 50) {
            loadMessages()
        }
    }, 30))
}

document.getElementById("sendButton").onclick = async () => {
    const content = getInputValue("messageInput")

    if (!selectedRecipient || !content) {
        alert("Select a user and type a message.");
        return;
    }

    const msg = {
        from: currentUser,
        to: selectedRecipient,
        content: content,
        timestamp: new Date().toISOString()
    };

    sock.send(JSON.stringify(msg));

    const chatBox = document.getElementById("chatBox")
    chatBox.appendChild(createMessageElement(msg));
    chatBox.scrollTop = chatBox.scrollHeight

    document.getElementById("messageInput").value = "";

    // Refresh user list to update sorting
    loadUsers();
};

async function loadUsers() {
    const res = await fetch(`/users?currentUser=${currentUser}`);
    const users = await res.json();
    populateUserList(users)
}

// session check
window.onload = () => {
    const savedUser = localStorage.getItem("chat_user");
    if (savedUser) {
        currentUser = savedUser;
        startChat();
    }
};