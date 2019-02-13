import Rx from 'rxjs/Rx';
import amqp from 'amqplib';
import retryp from 'promise-retry';

import to from './to';

const retryOpts = { retries: 3, maxTimeout: 2 * 1000 };

export class AmqpClient {
  static connect(endpoint, client) {
    let err = undefined, conn = undefined;
    client._haltFlow();
    (async function () {
      function doConnect(retry) {
        return amqp.connect(endpoint).catch(retry);
      }
      [ err, conn ] = await to(retryp(doConnect, retryOpts));
      if (err) {
        console.error(`AmqpClient::connect - ${err.message} - ${err.stack}`);
        AmqpClient.connect(endpoint, client);
        return;
      }
      client._setConn(conn, endpoint);
    })()
    return Promise.resolve(client);
  }

  constructor(opts = {}) {
    // Channel
    this.chan$ = new Rx.BehaviorSubject(null);

    // Destinations
    this._queue = new Map();
    this._exchange = new Map();

    // Options
    this._prefetch = opts.prefetch || 1;
  }

  async _setConn(conn, endpoint) {
    conn.on("error", (err) => {
      if (err.message !== "Connection closing") {
        console.error(`AmqpClient::_setConn - ${err.message} - ${err.stack}`);
      }
    });

    conn.on("close", () => {
      console.error(`AmqpClient::_setConn - close`);
      AmqpClient.connect(endpoint, this);
    });

    let err = undefined, channel = undefined;

    [ err, channel ] = await to(conn.createConfirmChannel());
    if (err) {
      console.error(`AmqpClient::_setConn - ${err.message} - ${err.stack}`);
      return;
    }

    [ err ] = await to(channel.prefetch(this._prefetch));
    if (err) {
      console.error(`AmqpClient::_setConn - ${err.message} - ${err.stack}`);
      return;
    }

    this._resumeFlow(channel);
  }

  _resumeFlow(chan) {
    this.chan$.next(chan);
    console.log(`AmqpClient::_resumeFlow`);
  }

  _haltFlow() {
    this.chan$.next(null);
    console.log(`AmqpClient::_haltFlow`);
  }

  _prepareLocalBuffer() {
    // todo: implement better persistent based solution for messaging in holding area
    let bufferMsg = [];
    return bufferMsg;
  }

  _prepareQueue({ name, reply = false, opts = {} }) {
    let err = undefined;

    if (this._queue.has(name)) {
      return this._queue.get(name);
    }

    let bufferMsg = this._prepareLocalBuffer();

    // Prepare pipeline
    let incoming$ = new Rx.Subject();

    Rx.Observable.merge(this.chan$, incoming$)
      .do(event => {
        console.log(`AmqpClient::pipeline - ${name}::queue - ${event}`);
      })
      .filter(event => {
        if (event === null) {
          return false; // channel closed, suspend publishing
        } else {
          if (event instanceof Array) {
            bufferMsg.push(event);
          }
          // In order to progress, the broker must be available and has at
          // least one message to go
          return (this.chan$.getValue() !== null);
        }
      })
      .concatMap(async (event) => {
        if ('assertQueue' in event && reply === false) {
          let channel = event;
          [ err ] = await to(channel.assertQueue(name, opts));
          if (err) {
            console.log(`AmqpClient::pipeline - ${name}::queue::create::err - ${err.message} - ${err.stack}`);
            return Promise.reject(err);
          }
          return [ channel, bufferMsg ];
        }
        return [ this.chan$.getValue(), bufferMsg ];
      })
      .concatMap(async (result) => {
        let [ channel, pendingMsg ] = result, counter = 0;
        while (pendingMsg.length > 0) {
          let mesg = pendingMsg.shift();
          [ err ] = await to(channel.sendToQueue(name, ...mesg));
          if (err) {
            pendingMsg.unshift(mesg);
            console.log(`AmqpClient::pipeline - ${name}::queue::sendToQueue::err - ${err.message} - ${err.stack}`);
            return Promise.reject(err);
          }
          counter++;
        }
        return counter;
      })
      .catch((err, caught) => {
        console.error(`AmqpClient::pipeline - ${name}::queue::*::err - ${err.message} - ${err.stack}`);
        return caught.delay(1000);
      })
      .forEach(counter => {
        console.log(`AmqpClient::pipeline - ${name}::queue - processing (${counter})`);
      });

    this._queue.set(name, incoming$);
    return incoming$;
  }

  _prepareExchange({ name, type = 'direct', opts = {} }) {
    let err = undefined;

    if (this._exchange.has(name)) {
      return this._exchange.get(name);
    }

    let bufferMsg = this._prepareLocalBuffer();

    // Prepare pipeline
    let incoming$ = new Rx.Subject();

    Rx.Observable.merge(this.chan$, incoming$)
      .do(event => {
        console.log(`AmqpClient::pipeline - ${name}::exchange - ${event}`);
      })
      .filter(event => {
        if (event === null) {
          return false; // channel closed, suspend publishing
        } else {
          if (event instanceof Array) {
            bufferMsg.push(event);
          }
          // In order to progress, the broker must be available and has at
          // least one message to go
          return (this.chan$.getValue() !== null);
        }
      })
      .concatMap(async (event) => {
        if ('assertExchange' in event) {
          let channel = event;
          [ err ] = await to(channel.assertExchange(name, type, opts));
          if (err) {
            console.error(`AmqpClient::pipeline - ${name}::exchange::create::err - ${err.message} - ${err.stack}`);
            return Promise.reject(err);
          }
          return [ channel, bufferMsg ];
        }
        return [ this.chan$.getValue(), bufferMsg ];
      })
      .concatMap(async (result) => {
        let [ channel, pendingMsg ] = result, counter = 0;
        while (pendingMsg.length > 0) {
          let mesg = pendingMsg.shift();
          [ err ] = await to(channel.publish(name, ...mesg));
          if (err) {
            pendingMsg.unshift(mesg);
            console.error(`AmqpClient::pipeline - ${name}::exchange::publish::err - ${err.message} - ${err.stack}`);
            return Promise.reject(err);
          }
          counter++;
        }
        return counter;
      })
      .catch((err, caught) => {
        console.error(`AmqpClient::pipeline - ${name}::exchange::*::err - ${err.message} - ${err.stack}`);
        return caught.delay(500);
      })
      .forEach(counter => {
        console.log(`AmqpClient::pipeline - ${name}::exchange - processing (${counter})`);
      });

    this._exchange.set(name, incoming$);
    return incoming$;
  }

  commit(payload, { exchange, queue, routing_key = '' }, mesgOpts = { persistent: true }) {
    let target$ = undefined;
    let message = Buffer.from(JSON.stringify(payload));
    if (queue) {
      target$ = this._prepareQueue(queue);
      return target$.next([ message, mesgOpts ]);
    }
    if (exchange) {
      target$ = this._prepareExchange(exchange);
      return target$.next([ routing_key, message, mesgOpts ]);
    }
    throw new Error('error/invalid-publish-options');
  }

  each({ exchange, queue, routing_key, noAck, json = true }) {
    return this.chan$.filter(channel => channel !== null)
      .concatMap(async (channel) => {
        let err = undefined, que = undefined;
        if (queue) {
          let { name = '', opts } = queue;
          [ err, que ] = await to(channel.assertQueue(name, opts));
          if (err) {
            console.error(`AmqpClient::each - queue::err - ${err.message} - ${err.stack}`);
            return Promise.reject(err);
          }
          console.log(`AmqpClient::each - queue`, que);
        }
        if (exchange) {
          let { name, type, opts } = exchange;
          [ err ] = await to(channel.assertExchange(name, type, opts));
          if (err) {
            console.error(`AmqpClient::each - exchange::err - ${err.message} - ${err.stack}`);
            return Promise.reject(err);
          }
          console.log(`AmqpClient::each - exchange`, name, type);
          if (routing_key) {
            [ err ] = await to(Promise.all(routing_key.map(k => channel.bindQueue(que.queue, name, k))));
            if (err) {
              console.error(`AmqpClient::each - bind::routing_key::err - ${err.message} - ${err.stack}`);
              return Promise.reject(err);
            }
            console.log(`AmqpClient::each - exchange - routing_key`, name, que.queue, routing_key);
          } else {
            [ err ] = await to(channel.bindQueue(que.queue, name, ''));
            if (err) {
              console.error(`AmqpClient::each - bind::no_routing_key::err - ${err.message} - ${err.stack}`);
              return Promise.reject(err);
            }
            console.log(`AmqpClient::each - exchange - direct`, name, que.queue, routing_key);
          }
        }
        return [ channel, que ];
      })
      .catch((err, caught) => {
        console.error(`AmqpClient::each - *:err - ${err.message} - ${err.stack}`);
        return caught.delay(2000);
      })
      .switchMap(([ channel, que ]) => {
        const sub$ = new Rx.Subject();
        const consumeOpts = {
          noAck: noAck
        };
        channel.consume(que.queue, (msg) => {
          sub$.next({
            message: (json === true ? JSON.parse(msg.content.toString()) : msg.content),
            meta: {
              fields: msg.fields,
              properties: msg.properties
            },
            ack: ({ allUpTo = false } = {}) => {
              console.log(`AmqpClient::each - ack - ${allUpTo}`);
              return channel.ack(msg, allUpTo)
            },
            nack: ({ allUpTo = false, requeue = false } = {}) => {
              console.log(`AmqpClient::each - nack - ${allUpTo} - ${requeue}`);
              return channel.nack(msg, allUpTo, requeue)
            }
          });
        }, consumeOpts);
        return sub$;
      });
  }
}
