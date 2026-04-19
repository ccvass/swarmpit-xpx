package swarmpit.socket;

import org.apache.http.HttpHost;
import org.apache.http.conn.socket.ConnectionSocketFactory;
import org.apache.http.protocol.HttpContext;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.nio.file.Path;

public class UnixSocketFactory implements ConnectionSocketFactory {

    private final Path socketPath;

    private UnixSocketFactory(String file) {
        this.socketPath = Path.of(file);
    }

    public static UnixSocketFactory createUnixSocketFactory(String socket) {
        return new UnixSocketFactory(socket);
    }

    @Override
    public Socket createSocket(HttpContext context) throws IOException {
        // Socket connects to Unix domain socket in constructor
        return new HttpUnixSocket(socketPath);
    }

    @Override
    public Socket connectSocket(int connectTimeout, Socket sock, HttpHost host,
                                InetSocketAddress remoteAddress, InetSocketAddress localAddress,
                                HttpContext context) throws IOException {
        // Already connected
        return sock;
    }
}
