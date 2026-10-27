import { Link, Navigate, useNavigate } from 'react-router';

import { client } from '../../api/client';
import { useAuth } from '../../auth/useAuth';
import { AuthForm } from '../../components/AuthForm/AuthForm';
import { unwrap } from '../../hooks/apiError';

export function Register() {
  const { user, signIn } = useAuth();
  const navigate = useNavigate();

  if (user) {
    return <Navigate to="/" replace />;
  }

  const submit = async (login: string, password: string) => {
    const session = unwrap(await client.POST('/api/auth/register', { body: { login, password } }));
    signIn(session);
    void navigate('/', { replace: true });
  };

  return (
    <AuthForm
      heading="Регистрация"
      submitLabel="Зарегистрироваться"
      checkPasswordBytes
      newPassword
      footer={<Link to="/login">Вход</Link>}
      onSubmit={submit}
    />
  );
}
