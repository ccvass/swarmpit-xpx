package swarmpit.socket;

import org.apache.http.HttpHost;
import org.apache.http.conn.ConnectTimeoutException;
import org.apache.http.conn.socket.ConnectionSocketFactory;
import org.apache.http.protocol.HttpContext;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.SocketTimeoutException;
import java.net.UnixDomainSocketAddress;
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
        return new HttpUnixSocket(socketPath);
    }

    @Override
    public Socket connectSocket(int connectTimeout, Socket sock, HttpHost host,
                                InetSocketAddress remoteAddress, InetSocketAddress localAddress,
                                HttpContext context) throws IOException {
        try {
            sock.connect(UnixDomainSocketAddress.of(socketPath), connectTimeout);
        } catch (SocketTimeoutException e) {
            throw new ConnectTimeoutException(e, null, remoteAddress.getAddress());
        }
        return sock;
    }
}
