const net = require('net');
const availableNicknames = require('./nicknames.json');

const nicknames = new WeakMap();

function getRandomInt(min, max) {
  min = Math.ceil(min);
  max = Math.floor(max);
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function getRandomNickname() {
  return availableNicknames[getRandomInt(0, availableNicknames.length - 1)];
}

function log(msg) {
  console.log(new Date().toLocaleTimeString(), '-', msg);
}

function sendToClient(socket, msg, cb = () => {}) {
  socket.write(`>> ${msg}\n`, cb);
}

const clients = {
  broadcast: function sendToAll(msg, fromSocket) {
    let from = '[SERVER]';

    if (fromSocket) {
      const nick = nicknames.get(fromSocket);
      from = `${nick}@${fromSocket.remoteAddress}:${fromSocket.remotePort}`;
    }

    const finalMsg = `${from} > ${msg}`;

    clients._connected.forEach(socket =>
      sendToClient(
        socket,
        finalMsg,
      )
    );
  },
  add: function addClientToList(socket, nick) {
    clients._connected.push(socket);
    nicknames.set(socket, nick);
  },
  remove: function removeClientFromList(socket) {
    const index = clients._connected.find(s => s === socket);

    if (index !== -1) {
      clients._connected.splice(index, 1);
    }
  },
  _connected: [],
};

const commands = {
  EXIT: ({ socket, ip, port }) => {
    sendToClient(socket, 'bye!', () => {
      socket.end();
    });
  },
  SAY: ({ msg, socket, ip, port }) => {
    clients.broadcast(msg, socket);
  },
  LIST: ({ socket, ip, port }) => {
    const everybody = clients._connected.map(socket => nicknames.get(socket));

    sendToClient(
      socket,
      `${clients._connected.length} clients connected: ${everybody.join(', ')}.`
    );
  },
  HELP: ({ socket, ip, port }) => {
    sendToClient(socket, 'Available commands: HELP, LIST, EXIT, SAY.');
  }
};

const msgRe = /^HELP$|^EXIT$|^LIST$|^(SAY) (.*)/;

const server = net.createServer((socket) => {
  socket.setKeepAlive(true);
  // optimize latency vs throughput
  socket.setNoDelay(true);

  const ip = socket.remoteAddress;
  const port = socket.remotePort;
  const nick = getRandomNickname();

  clients.add(socket, nick);
  clients.broadcast(`${nick}@${ip}:${port} has joined the building`);
  sendToClient(socket, "Hello there, friend!");
  commands.HELP({ socket });

  let buf = '';

  socket.on('data', function onServerRead(data) {
    let msg = data.toString('utf8');
    log(`<< ${ip}:${port} : ${msg}`);

    // buffering the message until we get the \n
    // that signals the end of the message
    if (msg.indexOf('\n') === -1) {
      buf += msg;
      return;
    } else if (buf !== '') {
      msg = `${buf}${msg}`;
      buf = '';
    }

    msg = msg.trim().replace(/\n$/, '');

    let cmd = 'HELP';

    const match = msg.match(msgRe);

    if (match !== null) {
      if (['HELP', 'LIST', 'EXIT'].includes(match[0])) {
        cmd = match[0];
        msg = match[1];
      } else if (['SAY'].includes(match[1])) {
        cmd = match[1];
        msg = match[2];
      }
    }

    commands[cmd]({ socket, msg, ip, port });
  });

  socket.on('error', (err) => {
    clients.remove(socket);

    if (/(ECONNRESET|EPIPE)/.test(err.message)) {
      log(`Connection reset by client ${ip}:${port}`);
    } else {
      log(`Client ${ip}:${port} error: ${err.stack}`);
    }

    clients.broadcast(`${nick}@${ip}:${port} has left the building`);
  });

  socket.on('end', () => {
    clients.remove(socket);
    log(`Client ${ip}:${port} disconnected`);
    clients.broadcast(`${nick}@${ip}:${port} has left the building`);
  });
});

server.on('error', (err) => {
  // crash server
  throw err;
});

server.listen(9999, () => {
  log(`Server listening on port ${server.address().port}`);
});
